package pythpro

import (
	"context"
	"encoding/binary"
	stderrors "errors"
	"fmt"
	"math/big"
	"strconv"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// Keeper is the subset of keeper methods used by the PythPro assistant.
type Keeper interface {
	Logger(ctx sdk.Context) log.Logger
	Meter(ctx context.Context) metrics.Meter
	GetParams(ctx sdk.Context) types.Params
	CallEVMWithValue(ctx sdk.Context, from common.Address, contractAddr common.Address, data []byte, gasCap uint64, value *big.Int) ([]byte, error)
	GetPythProPriceState(ctx sdk.Context, feedID uint32) *types.PythProPriceState
	SetPythProPriceState(ctx sdk.Context, priceState *types.PythProPriceState)
	EmitPythProPriceUpdate(ctx sdk.Context, feedID uint32, priceState *types.PriceState)
}

// Binary layout matches PythLazerLib / PythLazerStructs (EVM).
const (
	payloadMagic     uint32 = 2479346549 // 0x93A7B6D5
	payloadHeaderLen int    = 14         // 4 magic + 8 ts + 1 channel + 1 feedsLen
)

const (
	propPrice               byte = iota // 0
	propBestBidPrice                    // 1
	propBestAskPrice                    // 2
	propPublisherCount                  // 3
	propExponent                        // 4
	propConfidence                      // 5
	propFundingRate                     // 6
	propFundingTimestamp                // 7
	propFundingRateInterval             // 8
	propMarketSession                   // 9
	propEmaPrice                        // 10
	propEmaConfidence                   // 11
	propFeedUpdateTimestamp             // 12
	propMax                 = propFeedUpdateTimestamp
)

var (
	verifyUpdateMethod abi.Method
)

func init() {
	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil {
		panic("failed to create bytes ABI type: " + err.Error())
	}
	addressType, err := abi.NewType("address", "", nil)
	if err != nil {
		panic("failed to create address ABI type: " + err.Error())
	}

	verifyUpdateMethod = abi.NewMethod(
		"verifyUpdate", "verifyUpdate", abi.Function, "", false, true,
		abi.Arguments{{Name: "update", Type: bytesType}},
		abi.Arguments{{Name: "payload", Type: bytesType}, {Name: "signer", Type: addressType}},
	)
}

// parsedPayload holds the decoded result of a Pyth Lazer payload.
type parsedPayload struct {
	timestamp uint64 // batch header timestamp (μs)
	feeds     []feedPrice
}

// feedPrice is the price extracted from a single feed in the payload.
type feedPrice struct {
	feedID              uint32
	price               int64
	exponent            int16
	feedUpdateTimestamp *uint64 // nil → use parsedPayload.timestamp
}

// parsedProperty is the optional decoded fields from one wire property in a feed.
type parsedProperty struct {
	priceMantissa *int64
	exponent      *int16
	feedTS        *uint64
}

// applyExponent returns mantissa × 10^exponent as LegacyDec (no floating point).
// The power of 10 is computed as a math.Int via big.Int.Exp so there is no loop
// and no LegacyDec multiplication accumulation.
func applyExponent(mantissa int64, exp int16) math.LegacyDec {
	m := math.LegacyNewDec(mantissa)
	if exp == 0 {
		return m
	}
	absE := int(exp)
	if exp < 0 {
		absE = -int(exp)
	}
	pow10 := math.NewIntFromBigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absE)), nil))
	if exp > 0 {
		return m.MulInt(pow10)
	}
	return m.QuoInt(pow10)
}

//nolint:revive // Cursor-based parser helper returns value, cursor, presence, and error by design.
func consumeOptionalExistsUint64(data []byte, pos int) (val uint64, newPos int, ok bool, err error) {
	if pos+1 > len(data) {
		return 0, pos, false, fmt.Errorf("pyth pro payload truncated reading exists flag at byte %d", pos)
	}
	exists := data[pos]
	pos++
	if exists == 0 {
		return 0, pos, false, nil
	}
	if pos+8 > len(data) {
		return 0, pos, false, fmt.Errorf("pyth pro payload truncated reading uint64 after exists at byte %d", pos)
	}
	v := binary.BigEndian.Uint64(data[pos : pos+8])
	return v, pos + 8, true, nil
}

// parsePythProPayload decodes a Pyth Lazer EVM binary payload (post-verifyUpdate).
// Format follows PythLazerLib.parseUpdateFromPayload (big-endian).
//
//nolint:revive // Binary protocol parser intentionally keeps explicit branching for readability and safety.
func parsePythProPayload(data []byte) (parsedPayload, error) {
	if len(data) < payloadHeaderLen {
		return parsedPayload{}, fmt.Errorf("pyth pro payload too short: got %d bytes, need at least %d", len(data), payloadHeaderLen)
	}
	if binary.BigEndian.Uint32(data[0:4]) != payloadMagic {
		return parsedPayload{}, stderrors.New("pyth pro payload: invalid magic")
	}
	headerTS := binary.BigEndian.Uint64(data[4:12])
	// data[12] channel — unused
	feedsLen := int(data[13])
	pos := payloadHeaderLen

	feeds := make([]feedPrice, 0, feedsLen)
	for i := range feedsLen {
		if pos+5 > len(data) {
			return parsedPayload{}, fmt.Errorf("pyth pro payload truncated at feed %d: expected feedId+numProperties", i)
		}
		feedID := binary.BigEndian.Uint32(data[pos : pos+4])
		numProperties := int(data[pos+4])
		pos += 5

		var (
			priceMantissa *int64
			exponent      *int16
			feedTS        *uint64
		)

		for j := range numProperties {
			if pos >= len(data) {
				return parsedPayload{}, fmt.Errorf("pyth pro payload truncated at feed %d property %d (property id)", i, j)
			}
			propID := data[pos]
			pos++
			if propID > propMax {
				return parsedPayload{}, fmt.Errorf("pyth pro payload: unknown property id %d at feed %d property %d", propID, i, j)
			}

			var out parsedProperty
			var err error
			out, pos, err = consumeProperty(propID, data, pos, i, j)
			if err != nil {
				return parsedPayload{}, err
			}
			if out.priceMantissa != nil {
				priceMantissa = out.priceMantissa
			}
			if out.exponent != nil {
				exponent = out.exponent
			}
			if out.feedTS != nil {
				feedTS = out.feedTS
			}
		}

		if priceMantissa == nil || exponent == nil {
			continue
		}
		if *priceMantissa <= 0 {
			continue
		}

		feeds = append(feeds, feedPrice{
			feedID:              feedID,
			price:               *priceMantissa,
			exponent:            *exponent,
			feedUpdateTimestamp: feedTS,
		})
	}

	if pos != len(data) {
		return parsedPayload{}, fmt.Errorf("pyth pro payload: unconsumed trailing bytes (pos=%d len=%d)", pos, len(data))
	}

	return parsedPayload{timestamp: headerTS, feeds: feeds}, nil
}

//nolint:revive // Property decoding must branch per wire-type to preserve protocol-level clarity.
func consumeProperty(propID byte, data []byte, pos, feedIdx, propIdx int) (out parsedProperty, newPos int, err error) {
	switch propID {
	case propPrice:
		if pos+8 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (price value)", feedIdx, propIdx)
		}
		v := int64(binary.BigEndian.Uint64(data[pos : pos+8]))
		pos += 8
		out.priceMantissa = &v
	case propBestBidPrice, propBestAskPrice, propEmaPrice:
		if pos+8 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (int64 property %d)", feedIdx, propIdx, propID)
		}
		pos += 8
	case propPublisherCount, propMarketSession:
		if pos+2 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (int16/uint16 property %d)", feedIdx, propIdx, propID)
		}
		pos += 2
	case propExponent:
		if pos+2 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (exponent)", feedIdx, propIdx)
		}
		v := int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
		out.exponent = &v
	case propConfidence, propEmaConfidence:
		if pos+8 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (uint64 property %d)", feedIdx, propIdx, propID)
		}
		pos += 8
	case propFundingRate:
		if pos+1 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (funding rate exists)", feedIdx, propIdx)
		}
		exists := data[pos]
		pos++
		if exists != 0 {
			if pos+8 > len(data) {
				return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (funding rate value)", feedIdx, propIdx)
			}
			pos += 8
		}
	case propFundingTimestamp, propFundingRateInterval:
		if pos+1 > len(data) {
			return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (funding ts/interval exists)", feedIdx, propIdx)
		}
		exists := data[pos]
		pos++
		if exists != 0 {
			if pos+8 > len(data) {
				return out, pos, fmt.Errorf("pyth pro payload truncated at feed %d property %d (funding ts/interval value)", feedIdx, propIdx)
			}
			pos += 8
		}
	case propFeedUpdateTimestamp:
		v, np, ok, err := consumeOptionalExistsUint64(data, pos)
		if err != nil {
			return out, pos, err
		}
		pos = np
		if ok {
			out.feedTS = &v
		}
	default:
		return out, pos, fmt.Errorf("pyth pro payload: unhandled property id %d", propID)
	}
	return out, pos, nil
}

// Assistant implements OracleAssistant for PythPro.
type Assistant struct {
	keeper Keeper
}

// NewAssistant constructs a PythPro assistant backed by the given keeper.
func NewAssistant(k Keeper) *Assistant {
	return &Assistant{keeper: k}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_PythPro
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.ProcessRelay")(&err)

	m, ok := msg.(*types.MsgRelayPythProPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayPythProPrices")
	}

	senderAddr := sdk.MustAccAddressFromBech32(m.Sender)
	from := common.BytesToAddress(senderAddr.Bytes())

	var (
		processed int
		lastErr   error
	)

	for _, update := range m.Updates {
		if err := a.processUpdate(ctx, update, from); err != nil {
			if stderrors.Is(err, types.ErrPythProVerificationFailed) {
				// If the batch includes at least one update that fails verification,
				// the entire batch is rejected (we consider this an attack vector)
				return err
			}
			lastErr = err
			continue
		}
		processed++
	}

	if processed == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

func (a *Assistant) processUpdate(ctx sdk.Context, update []byte, from common.Address) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.processUpdate")(&err)

	payload, err := a.verifyPythProUpdate(ctx, update, from)
	if err != nil {
		a.keeper.Logger(ctx).Error("PythPro update verification failed", "error", err)
		return err
	}

	parsed, err := parsePythProPayload(payload)
	if err != nil {
		a.keeper.Logger(ctx).Error("PythPro payload parse failed", "error", err)
		return errors.Wrap(types.ErrInvalidOracleRequest, err.Error())
	}

	blockTime := ctx.BlockTime().Unix()

	for _, fp := range parsed.feeds {
		ts := parsed.timestamp
		if fp.feedUpdateTimestamp != nil {
			ts = *fp.feedUpdateTimestamp
		}
		a.processFeedPrice(ctx, fp.feedID, fp.price, fp.exponent, ts, blockTime)
	}
	return nil
}

func (a *Assistant) processFeedPrice(ctx sdk.Context, feedID uint32, rawPrice int64, exponent int16, timestamp uint64, blockTime int64) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.processFeedPrice")()

	price := applyExponent(rawPrice, exponent)
	existing := a.keeper.GetPythProPriceState(ctx, feedID)

	if existing != nil {
		if timestamp <= existing.Timestamp {
			return
		}
		if types.CheckPriceFeedThreshold(existing.PriceState.Price, price) {
			return
		}
		existing.Update(price, timestamp, blockTime)
		a.keeper.SetPythProPriceState(ctx, existing)
	} else {
		newState := types.NewPythProPriceState(feedID, price, timestamp, blockTime)
		a.keeper.SetPythProPriceState(ctx, newState)
		existing = newState
	}

	a.keeper.EmitPythProPriceUpdate(ctx, feedID, &existing.PriceState)
}

func (a *Assistant) verifyPythProUpdate(ctx sdk.Context, update []byte, from common.Address) (payload []byte, err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.verifyPythProUpdate")(&err)

	params := a.keeper.GetParams(ctx)
	if params.PythProVerifierContract == "" {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "pyth pro verifier contract not configured")
	}

	verifierAddr := common.HexToAddress(params.PythProVerifierContract)

	callData, err := verifyUpdateMethod.Inputs.Pack(update)
	if err != nil {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "failed to encode verifyUpdate call data")
	}
	callDataWithSelector := make([]byte, 0, len(verifyUpdateMethod.ID)+len(callData))
	callDataWithSelector = append(callDataWithSelector, verifyUpdateMethod.ID...)
	callDataWithSelector = append(callDataWithSelector, callData...)

	value := new(big.Int).SetUint64(params.PythProVerificationFee)
	ret, err := a.keeper.CallEVMWithValue(ctx, from, verifierAddr, callDataWithSelector, params.PythProVerificationGasLimit, value)
	if err != nil {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, err.Error())
	}

	decoded, err := verifyUpdateMethod.Outputs.Unpack(ret)
	if err != nil {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "failed to decode verifyUpdate response")
	}
	if len(decoded) < 1 {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "unexpected verifyUpdate response size")
	}
	out, ok := decoded[0].([]byte)
	if !ok {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "unexpected verifyUpdate payload type")
	}
	if len(out) == 0 {
		return nil, errors.Wrap(types.ErrPythProVerificationFailed, "empty verifyUpdate payload")
	}

	return out, nil
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.PriceState")()
	feedID, err := parseFeedID(key)
	if err != nil {
		return nil
	}
	ps := a.keeper.GetPythProPriceState(ctx, feedID)
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_PythPro, scaling, quote) {
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
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "pythpro.Assistant.ReferencePrice")()

	baseFeedID, err := parseFeedID(base)
	if err != nil {
		return nil
	}
	basePriceState := a.keeper.GetPythProPriceState(ctx, baseFeedID)
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		return &basePriceState.PriceState.Price
	}

	quoteFeedID, err := parseFeedID(quote)
	if err != nil {
		return nil
	}
	quotePriceState := a.keeper.GetPythProPriceState(ctx, quoteFeedID)
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

// parseFeedID converts a string oracle key to a uint32 Pyth Lazer feed ID.
func parseFeedID(key string) (uint32, error) {
	id, err := strconv.ParseUint(key, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid PythPro feed ID %q: %w", key, err)
	}
	return uint32(id), nil
}
