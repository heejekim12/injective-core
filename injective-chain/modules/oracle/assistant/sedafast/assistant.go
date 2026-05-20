package sedafast

import (
	"context"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// Keeper is the subset of keeper methods used by the SedaFast assistant.
type Keeper interface {
	Logger(ctx sdk.Context) log.Logger
	Meter(ctx context.Context) metrics.Meter
	GetParams(ctx sdk.Context) types.Params
	GetSedaFastPriceState(ctx sdk.Context, feedID string) *types.SedaFastPriceState
	SetSedaFastPriceState(ctx sdk.Context, priceState *types.SedaFastPriceState)
	EmitSedaFastPriceUpdate(ctx sdk.Context, feedID string, priceState *types.PriceState)
}

// Assistant implements OracleAssistant for SedaFast.
type Assistant struct {
	keeper Keeper
}

// NewAssistant constructs a SedaFast assistant backed by the given keeper.
func NewAssistant(k Keeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_SedaFast
}

// ProcessRelay processes a MsgRelaySedaFastPrices. Signature verification
// failure aborts the whole batch. All other per-update errors (parse failure,
// unknown program, execution failure, price threshold) are best-effort: the
// failing update is skipped and processing continues.
func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "sedafast.Assistant.ProcessRelay")(&err)

	m, ok := msg.(*types.MsgRelaySedaFastPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelaySedaFastPrices")
	}

	params := a.keeper.GetParams(ctx)
	if len(params.SedaFastParams.PublicKey) == 0 {
		return types.ErrSedaFastDisabled
	}

	var (
		processed int
		lastErr   error
	)

	for _, raw := range m.Updates {
		if updateErr := a.processUpdate(ctx, raw, params.SedaFastParams); updateErr != nil {
			if stderrors.Is(updateErr, types.ErrSedaFastVerificationFailed) {
				// Signature failure → treat as byzantine relayer, abort batch.
				return updateErr
			}
			a.keeper.Logger(ctx).Warn("seda fast update skipped", "error", updateErr)
			lastErr = updateErr
			continue
		}
		processed++
	}

	if processed == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

func (a *Assistant) processUpdate(ctx sdk.Context, raw []byte, sfp types.SedaFastParams) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "sedafast.Assistant.processUpdate")(&err)

	env, err := ParseEnvelope(raw)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, err.Error())
	}

	if err := validateExecutionStatus(env.Data.DataResult); err != nil {
		return err
	}

	// Resolve which price parser to use. Also enforces the program ID allowlist.
	parser, err := resolvePriceParser(env.Data.DataRequest.ExecProgramID, sfp)
	if err != nil {
		return err
	}

	// Verify message integrity and authenticity.
	if err = verifyUpdate(env, sfp); err != nil { //nolint:gocritic // intentional: = makes it explicit that the named return is updated; := would scope err to the if-block
		return err
	}

	// Parse price from data.dataResult.result — the field whose bytes are
	// covered by the dataResultId signature.  The unsigned top-level data.result
	// mirror is intentionally ignored; it is not part of the signed preimage and
	// must not influence on-chain state.
	resultBytes, err := decodeHexBytes(env.Data.DataResult.Result)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, "result hex decode: "+err.Error())
	}
	price, err := parser.Parse(resultBytes)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastParserFailed, err.Error())
	}

	if !price.IsPositive() {
		return errors.Wrapf(types.ErrBadPrice, "seda fast price must be positive, got %s", price)
	}

	feedID, err := canonicalizeFeedID(env.Data.DataRequest.FeedID)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, err.Error())
	}

	blockTimestamp, _ := strconv.ParseUint(env.Data.DataResult.BlockTimestamp, 10, 64)
	a.upsertPriceState(ctx, feedID, price, blockTimestamp, ctx.BlockTime().Unix())
	return nil
}

func (a *Assistant) upsertPriceState(ctx sdk.Context, feedID string, price math.LegacyDec, blockTimestamp uint64, blockTime int64) {
	existing := a.keeper.GetSedaFastPriceState(ctx, feedID)
	if existing != nil {
		if blockTimestamp <= existing.Timestamp {
			return
		}
		if types.CheckPriceFeedThreshold(existing.PriceState.Price, price) {
			return
		}
		existing.Update(price, blockTimestamp, blockTime)
	} else {
		existing = types.NewSedaFastPriceState(feedID, price, blockTimestamp, blockTime)
	}
	a.keeper.SetSedaFastPriceState(ctx, existing)
	a.keeper.EmitSedaFastPriceUpdate(ctx, feedID, &existing.PriceState)
}

// verifyUpdate performs the drId integrity check and the secp256k1 signature
// verification. It returns ErrSedaFastPayloadMalformed for integrity failures
// (best-effort skip) and ErrSedaFastVerificationFailed for signature failures
// (batch abort).
func verifyUpdate(env *Envelope, sfp types.SedaFastParams) error {
	derivedDrID, err := env.Data.DataRequest.DrID()
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, "drId derivation: "+err.Error())
	}
	reportedDrID, err := decodeHex32(env.Data.DataResult.DrID)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, "drId decode: "+err.Error())
	}
	if derivedDrID != reportedDrID {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed,
			fmt.Sprintf("drId mismatch: computed %x, reported %s", derivedDrID, env.Data.DataResult.DrID))
	}

	dataResultID, err := env.Data.DataResult.DataResultID()
	if err != nil {
		return errors.Wrap(types.ErrSedaFastPayloadMalformed, "dataResultId derivation: "+err.Error())
	}
	sigBytes, err := decodeHexBytes(env.Data.Signature)
	if err != nil {
		return errors.Wrap(types.ErrSedaFastVerificationFailed, "signature decode: "+err.Error())
	}
	if verifyErr := VerifySignature(sfp.PublicKey, sigBytes, dataResultID); verifyErr != nil {
		return errors.Wrap(types.ErrSedaFastVerificationFailed, verifyErr.Error())
	}
	return nil
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "sedafast.Assistant.PriceState")()
	ps := a.keeper.GetSedaFastPriceState(ctx, key)
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "sedafast.Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_SedaFast, scaling, quote) {
		return nil
	}

	basePriceState := a.PriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		baseRate := basePriceState.Price
		if baseRate.IsNil() || !baseRate.IsPositive() {
			return nil
		}
		return shared.PricePairStateForUSD(*basePriceState, baseRate)
	}

	quotePriceState := a.PriceState(ctx, quote)
	return shared.CombinePairPriceState(basePriceState, quotePriceState, quote, scaling)
}

func (a *Assistant) ReferencePrice(ctx sdk.Context, base, quote string) *math.LegacyDec {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "sedafast.Assistant.ReferencePrice")()

	basePriceState := a.keeper.GetSedaFastPriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		return &basePriceState.PriceState.Price
	}

	quotePriceState := a.keeper.GetSedaFastPriceState(ctx, quote)
	if quotePriceState == nil {
		return nil
	}

	basePrice := basePriceState.PriceState.Price
	quotePrice := quotePriceState.PriceState.Price
	if basePrice.IsNil() || quotePrice.IsNil() || !basePrice.IsPositive() || !quotePrice.IsPositive() {
		return nil
	}

	price := basePrice.Quo(quotePrice)
	return &price
}

// --- price parser ---

// priceParser parses the raw UTF-8 result bytes from data.result into a price.
type priceParser interface {
	Parse(raw []byte) (math.LegacyDec, error)
}

// simplePriceParser handles feeds in simple_program_ids: the result bytes are
// an ASCII decimal string (e.g. "384.48255").
type simplePriceParser struct{}

func (simplePriceParser) Parse(raw []byte) (math.LegacyDec, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return math.LegacyDec{}, stderrors.New("seda fast simple result: empty")
	}
	d, err := math.LegacyNewDecFromStr(s)
	if err != nil {
		return math.LegacyDec{}, fmt.Errorf("seda fast simple result %q: %w", s, err)
	}
	return d, nil
}

// jsonPriceParser handles feeds in json_program_ids: the result bytes are a
// UTF-8 JSON object {price:{mantissa,expo}}.
type jsonPriceParser struct{}

type jsonPriceData struct {
	Mantissa string `json:"mantissa"`
	Expo     int16  `json:"expo"`
}

type jsonResultPayload struct {
	Price jsonPriceData `json:"price"`
}

func (jsonPriceParser) Parse(raw []byte) (math.LegacyDec, error) {
	if len(raw) > types.MaxSedaFastUpdateSize {
		return math.LegacyDec{}, fmt.Errorf("seda fast json result too large: %d bytes", len(raw))
	}
	var payload jsonResultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return math.LegacyDec{}, fmt.Errorf("seda fast json result: %w", err)
	}
	if payload.Price.Mantissa == "" {
		return math.LegacyDec{}, stderrors.New("seda fast json result: price.mantissa is empty")
	}
	if payload.Price.Expo < -types.MaxSedaFastExponent || payload.Price.Expo > types.MaxSedaFastExponent {
		return math.LegacyDec{}, fmt.Errorf("seda fast json result: exponent %d out of range [-%d, %d]",
			payload.Price.Expo, types.MaxSedaFastExponent, types.MaxSedaFastExponent)
	}
	mantissa, ok := new(big.Int).SetString(payload.Price.Mantissa, 10)
	if !ok {
		return math.LegacyDec{}, fmt.Errorf("seda fast json result: invalid mantissa %q", payload.Price.Mantissa)
	}
	if mantissa.BitLen() > types.MaxSedaFastMantissaBits {
		return math.LegacyDec{}, fmt.Errorf("seda fast json result: mantissa bit length %d exceeds limit %d",
			mantissa.BitLen(), types.MaxSedaFastMantissaBits)
	}
	return applyExponent(math.LegacyNewDecFromBigInt(mantissa), payload.Price.Expo), nil
}

// validateExecutionStatus rejects envelopes with invalid or failed execution.
// exitCode must fit in a byte because DataResultID() only commits byte(exitCode):
// a relayer could present exitCode=256 (which hashes to 0) and rewrite it to 0
// without invalidating the signature. Non-byte exit codes are therefore malformed.
func validateExecutionStatus(res DataResult) error {
	if res.ExitCode > 0xFF {
		return errors.Wrapf(types.ErrSedaFastPayloadMalformed,
			"exitCode %d does not fit in a byte", res.ExitCode)
	}
	if res.ExitCode != 0 || !res.Consensus {
		return errors.Wrapf(types.ErrSedaFastExecutionFailed,
			"exitCode=%d consensus=%v", res.ExitCode, res.Consensus)
	}
	return nil
}

// applyExponent returns mantissa × 10^exp as a LegacyDec without floating
// point. Uses LegacyDec.Power (binary exponentiation over exact integers).
// exp is widened to int32 before negation to avoid int16 overflow on MinInt16.
func applyExponent(mantissa math.LegacyDec, exp int16) math.LegacyDec {
	if mantissa.IsZero() || exp == 0 {
		return mantissa
	}
	absE := max(int32(exp), -int32(exp))
	pow10 := math.LegacyNewDec(10).Power(uint64(absE))
	if exp > 0 {
		return mantissa.Mul(pow10)
	}
	return mantissa.Quo(pow10)
}

// resolvePriceParser returns the price parser for execProgramID by looking it
// up in the simple_program_ids and json_program_ids allowlists. Comparison is
// exact (case-sensitive); program IDs in params are required to be lowercase hex.
func resolvePriceParser(execProgramID string, sfp types.SedaFastParams) (priceParser, error) {
	if slices.Contains(sfp.SimpleProgramIds, execProgramID) {
		return simplePriceParser{}, nil
	}
	if slices.Contains(sfp.JsonProgramIds, execProgramID) {
		return jsonPriceParser{}, nil
	}
	return nil, errors.Wrapf(types.ErrSedaFastProgramNotAllowed,
		"execProgramId %q not in simple_program_ids or json_program_ids", execProgramID)
}

// canonicalizeFeedID strips any 0x/0X prefix, hex-decodes, and re-encodes to
// lowercase. This normalises all wire variants of the same feed bytes to a
// single on-chain store key regardless of what the relayer sends.
func canonicalizeFeedID(raw string) (string, error) {
	stripped := strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
	b, err := hex.DecodeString(stripped)
	if err != nil || len(b) == 0 {
		return "", fmt.Errorf("feedId %q is not valid hex", raw)
	}
	return hex.EncodeToString(b), nil
}

func decodeHexBytes(s string) ([]byte, error) {
	if len(s) >= 2 && s[:2] == "0x" {
		s = s[2:]
	}
	if s == "" {
		return []byte{}, nil
	}
	return hex.DecodeString(s)
}

func decodeHex32(s string) ([32]byte, error) {
	b, err := decodeHexBytes(s)
	if err != nil {
		return [32]byte{}, err
	}
	if len(b) != 32 {
		return [32]byte{}, fmt.Errorf("expected 32 bytes, got %d", len(b))
	}
	return [32]byte(b), nil
}
