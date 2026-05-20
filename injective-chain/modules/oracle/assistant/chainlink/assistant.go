package chainlink

import (
	"context"
	stderrors "errors"
	"fmt"
	"math/big"

	"cosmossdk.io/errors"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/data-streams-sdk/go/feed"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant/shared"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

// DataStreamsKeeper is the subset of keeper methods used by the Chainlink Data Streams assistant.
type DataStreamsKeeper interface {
	Logger(ctx sdk.Context) log.Logger
	Meter(ctx context.Context) metrics.Meter
	GetParams(ctx sdk.Context) types.Params
	CallEVM(ctx sdk.Context, contractAddr common.Address, data []byte, gasCap uint64) ([]byte, error)
	ProcessChainlinkDataStreamsReport(
		ctx sdk.Context,
		feedID string,
		reportPrice math.Int,
		validFromTimestamp, observationsTimestamp, expiresAt uint64,
		price math.LegacyDec,
	)
	GetChainlinkDataStreamsPriceState(ctx sdk.Context, feedID string) *types.ChainlinkDataStreamsPriceState
}

var (
	bytesType    abi.Type
	verifyMethod abi.Method

	reportBytes32Type abi.Type
	reportUint32Type  abi.Type
	reportUint64Type  abi.Type
	reportUint192Type abi.Type
	reportInt192Type  abi.Type

	v3ReportArgs abi.Arguments
	v8ReportArgs abi.Arguments
)

func init() {
	var err error
	bytesType, err = abi.NewType("bytes", "bytes", nil)
	if err != nil {
		panic("failed to create bytes ABI type: " + err.Error())
	}

	verifyMethod = abi.NewMethod("verify", "verify", abi.Function, "", false, true,
		abi.Arguments{
			{Name: "payload", Type: bytesType},
			{Name: "parameterPayload", Type: bytesType},
		},
		abi.Arguments{{Type: bytesType}},
	)

	reportBytes32Type, err = abi.NewType("bytes32", "bytes32", nil)
	if err != nil {
		panic("failed to create bytes32 ABI type: " + err.Error())
	}
	reportUint32Type, err = abi.NewType("uint32", "uint32", nil)
	if err != nil {
		panic("failed to create uint32 ABI type: " + err.Error())
	}
	reportUint64Type, err = abi.NewType("uint64", "uint64", nil)
	if err != nil {
		panic("failed to create uint64 ABI type: " + err.Error())
	}
	reportUint192Type, err = abi.NewType("uint192", "uint192", nil)
	if err != nil {
		panic("failed to create uint192 ABI type: " + err.Error())
	}
	reportInt192Type, err = abi.NewType("int192", "int192", nil)
	if err != nil {
		panic("failed to create int192 ABI type: " + err.Error())
	}

	v3ReportArgs = abi.Arguments{
		{Name: "feedId", Type: reportBytes32Type},
		{Name: "validFromTimestamp", Type: reportUint32Type},
		{Name: "observationsTimestamp", Type: reportUint32Type},
		{Name: "nativeFee", Type: reportUint192Type},
		{Name: "linkFee", Type: reportUint192Type},
		{Name: "expiresAt", Type: reportUint32Type},
		{Name: "price", Type: reportInt192Type},
		{Name: "bid", Type: reportInt192Type},
		{Name: "ask", Type: reportInt192Type},
	}

	v8ReportArgs = abi.Arguments{
		{Name: "feedId", Type: reportBytes32Type},
		{Name: "validFromTimestamp", Type: reportUint32Type},
		{Name: "observationsTimestamp", Type: reportUint32Type},
		{Name: "nativeFee", Type: reportUint192Type},
		{Name: "linkFee", Type: reportUint192Type},
		{Name: "expiresAt", Type: reportUint32Type},
		{Name: "lastUpdateTimestamp", Type: reportUint64Type},
		{Name: "midPrice", Type: reportInt192Type},
		{Name: "marketStatus", Type: reportUint32Type},
	}
}

type decodedReportData struct {
	feedIDStr             string
	price                 *big.Int
	validFromTimestamp    uint32
	observationsTimestamp uint32
	expiresAt             uint32
}

// Assistant implements OracleAssistant for Chainlink Data Streams.
type Assistant struct {
	keeper DataStreamsKeeper
}

// NewAssistant constructs a Chainlink Data Streams assistant backed by the given keeper.
func NewAssistant(k DataStreamsKeeper) *Assistant {
	return &Assistant{keeper: k}
}

func reportSchemaForVersion(feedVersion feed.FeedVersion) (abi.Arguments, int, error) {
	switch feedVersion {
	case feed.FeedVersion3:
		return v3ReportArgs, 6, nil
	case feed.FeedVersion8:
		return v8ReportArgs, 7, nil
	default:
		return nil, 0, fmt.Errorf("unsupported Chainlink Data Stream schema version: %d", feedVersion)
	}
}

func unpackDecodedReportFields(decoded []any, priceIndex int) (*decodedReportData, error) {
	feedIDBytes, err := bytes32ToBytes(decoded[0])
	if err != nil {
		return nil, err
	}
	validFromTimestamp, err := toUint32(decoded[1])
	if err != nil {
		return nil, err
	}
	observationsTimestamp, err := toUint32(decoded[2])
	if err != nil {
		return nil, err
	}
	expiresAt, err := toUint32(decoded[5])
	if err != nil {
		return nil, err
	}
	price, err := toBigInt(decoded[priceIndex])
	if err != nil {
		return nil, err
	}

	var decodedFeedID feed.ID
	copy(decodedFeedID[:], feedIDBytes)

	return &decodedReportData{
		feedIDStr:             decodedFeedID.String(),
		price:                 price,
		validFromTimestamp:    validFromTimestamp,
		observationsTimestamp: observationsTimestamp,
		expiresAt:             expiresAt,
	}, nil
}

func decodeVerifiedReport(feedID feed.ID, reportData []byte) (*decodedReportData, error) {
	args, priceIndex, err := reportSchemaForVersion(feedID.Version())
	if err != nil {
		return nil, err
	}

	decoded, err := args.Unpack(reportData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode verified report: %w", err)
	}
	if len(decoded) <= priceIndex {
		return nil, stderrors.New("unexpected verified report output size")
	}

	return unpackDecodedReportFields(decoded, priceIndex)
}

func (a *Assistant) decodeVerifiedReport(ctx sdk.Context, feedID feed.ID, reportData []byte) (_ *decodedReportData, err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.decodeVerifiedReport")(&err)
	return decodeVerifiedReport(feedID, reportData)
}

func bytes32ToBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case [32]byte:
		b := make([]byte, 32)
		copy(b, v[:])
		return b, nil
	case []byte:
		if len(v) != 32 {
			return nil, fmt.Errorf("unexpected bytes32 length: %d", len(v))
		}
		b := make([]byte, 32)
		copy(b, v)
		return b, nil
	default:
		return nil, fmt.Errorf("unexpected bytes32 type: %T", value)
	}
}

func toUint32(value any) (uint32, error) {
	switch v := value.(type) {
	case uint32:
		return v, nil
	case uint64:
		if v > uint64(^uint32(0)) {
			return 0, fmt.Errorf("uint32 overflow: %d", v)
		}
		return uint32(v), nil
	case *big.Int:
		if v.Sign() < 0 || v.BitLen() > 32 {
			return 0, fmt.Errorf("invalid uint32 value: %s", v.String())
		}
		return uint32(v.Uint64()), nil
	default:
		return 0, fmt.Errorf("unexpected uint32 type: %T", value)
	}
}

func toBigInt(value any) (*big.Int, error) {
	switch v := value.(type) {
	case *big.Int:
		return v, nil
	case big.Int:
		return &v, nil
	case int64:
		return big.NewInt(v), nil
	case uint64:
		return new(big.Int).SetUint64(v), nil
	case uint32:
		return new(big.Int).SetUint64(uint64(v)), nil
	default:
		return nil, fmt.Errorf("unexpected integer type: %T", value)
	}
}

func (*Assistant) OracleType() types.OracleType {
	return types.OracleType_ChainlinkDataStreams
}

func (a *Assistant) ProcessRelay(ctx sdk.Context, msg sdk.Msg) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ProcessRelay")(&err)
	m, ok := msg.(*types.MsgRelayChainlinkPrices)
	if !ok {
		return errors.Wrap(types.ErrInvalidOracleRequest, "expected MsgRelayChainlinkPrices")
	}
	if len(m.Reports) == 0 {
		return nil
	}

	var (
		processedCount int
		lastErr        error
	)

	for _, chainlinkReport := range m.Reports {
		if err := a.processChainlinkReport(ctx, chainlinkReport); err != nil {
			if stderrors.Is(err, types.ErrChainlinkVerificationFailed) {
				// If the batch includes at least one report that fails verification,
				// the entire batch is rejected (we consider this an attack vector)
				return err
			}
			lastErr = err
			continue
		}
		processedCount++
	}

	if processedCount == 0 && lastErr != nil {
		return lastErr
	}

	return nil
}

func (a *Assistant) processChainlinkReport(ctx sdk.Context, chainlinkReport *types.ChainlinkReport) (err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.processChainlinkReport")(&err)
	if chainlinkReport == nil || len(chainlinkReport.FeedId) == 0 || len(chainlinkReport.FullReport) == 0 {
		return errors.Wrap(types.ErrInvalidOracleRequest, "empty chainlink report")
	}

	var feedID feed.ID
	if len(chainlinkReport.FeedId) != 32 {
		a.keeper.Logger(ctx).Error("invalid feed ID length", "expected", 32, "got", len(chainlinkReport.FeedId))
		return errors.Wrap(types.ErrInvalidOracleRequest, "invalid feed ID length")
	}
	copy(feedID[:], chainlinkReport.FeedId)

	reportData, err := a.verifyChainlinkReport(ctx, chainlinkReport.FullReport)
	if err != nil {
		a.keeper.Logger(ctx).Error("Chainlink report verification failed", "error", err)
		return err
	}

	decoded, err := a.decodeVerifiedReport(ctx, feedID, reportData)
	if err != nil {
		a.keeper.Logger(ctx).Error("Chainlink report decode failed", "error", err)
		return errors.Wrap(types.ErrInvalidOracleRequest, err.Error())
	}

	expectedFeedID := feedID.String()
	if decoded.feedIDStr != expectedFeedID {
		a.keeper.Logger(ctx).Error("Chainlink report feed ID mismatch", "expected", expectedFeedID, "got", decoded.feedIDStr)
		return errors.Wrap(types.ErrInvalidOracleRequest, "feed ID mismatch")
	}

	if decoded.price == nil {
		a.keeper.Logger(ctx).Error("price is nil in decoded report")
		return errors.Wrap(types.ErrInvalidOracleRequest, "price is nil in decoded report")
	}

	priceDecimal := math.LegacyNewDecFromBigIntWithPrec(decoded.price, 18)

	if !priceDecimal.IsPositive() {
		a.keeper.Logger(ctx).Error("Chainlink report price is not positive", "feed_id", decoded.feedIDStr)
		return errors.Wrap(types.ErrInvalidOracleRequest, "price must be positive")
	}

	reportPriceInt := math.NewIntFromBigInt(decoded.price)

	a.keeper.ProcessChainlinkDataStreamsReport(
		ctx,
		decoded.feedIDStr,
		reportPriceInt,
		uint64(decoded.validFromTimestamp),
		uint64(decoded.observationsTimestamp),
		uint64(decoded.expiresAt),
		priceDecimal,
	)

	return nil
}

func (a *Assistant) verifyChainlinkReport(ctx sdk.Context, fullReport []byte) (verified []byte, err error) {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.verifyChainlinkReport")(&err)

	params := a.keeper.GetParams(ctx)

	if params.ChainlinkVerifierProxyContract == "" {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "verifier not configured")
	}

	verifierAddr := common.HexToAddress(params.ChainlinkVerifierProxyContract)

	callData, err := verifyMethod.Inputs.Pack(fullReport, []byte{})
	if err != nil {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "failed to encode call data")
	}

	callDataWithSelector := make([]byte, 0, len(verifyMethod.ID)+len(callData))
	callDataWithSelector = append(callDataWithSelector, verifyMethod.ID...)
	callDataWithSelector = append(callDataWithSelector, callData...)

	ret, err := a.keeper.CallEVM(ctx, verifierAddr, callDataWithSelector, params.ChainlinkDataStreamsVerificationGasLimit)
	if err != nil {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, err.Error())
	}

	decoded, err := verifyMethod.Outputs.Unpack(ret)
	if err != nil {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "failed to decode verifier response")
	}
	if len(decoded) != 1 {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "unexpected verifier response size")
	}
	out, ok := decoded[0].([]byte)
	if !ok {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "unexpected verifier response type")
	}

	if len(out) == 0 {
		return nil, errors.Wrap(types.ErrChainlinkVerificationFailed, "empty verified report")
	}

	return out, nil
}

func (a *Assistant) PriceState(ctx sdk.Context, key string) *types.PriceState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PriceState")()
	ps := a.keeper.GetChainlinkDataStreamsPriceState(ctx, key)
	if ps == nil {
		return nil
	}
	return &ps.PriceState
}

func (a *Assistant) PricePairState(ctx sdk.Context, base, quote string, scaling *types.ScalingOptions) *types.PricePairState {
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.PricePairState")()
	if !shared.PairScalingAllowed(types.OracleType_ChainlinkDataStreams, scaling, quote) {
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
	defer a.keeper.Meter(ctx).FuncTiming(&ctx, "Assistant.ReferencePrice")()
	basePriceState := a.keeper.GetChainlinkDataStreamsPriceState(ctx, base)
	if basePriceState == nil {
		return nil
	}

	if quote == types.QuoteUSD {
		return &basePriceState.PriceState.Price
	}

	quotePriceState := a.keeper.GetChainlinkDataStreamsPriceState(ctx, quote)
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
