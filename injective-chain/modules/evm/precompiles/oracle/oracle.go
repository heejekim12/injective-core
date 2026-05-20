package oracle

import (
	"errors"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/InjectiveLabs/metrics/v2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/precompiles"
	bindings "github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/precompiles/bindings/cosmos/precompile/oracle"
	precomptypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/evm/precompiles/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/assistant"
	oracletypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

const (
	OraclePriceMethodName                = "oraclePrice"
	OraclePricePairStateMethodName       = "oraclePricePairState"
	OraclePricePairStateScaledMethodName = "oraclePricePairStateScaled"
)

var (
	oracleABI                 abi.ABI
	oracleContractAddress     = common.HexToAddress("0x0000000000000000000000000000000000000067")
	oracleGasRequiredByMethod = map[[4]byte]uint64{}
)

var ErrPrecompilePanic = errors.New("precompile panic")

func init() {
	if err := oracleABI.UnmarshalJSON([]byte(bindings.OracleModuleMetaData.ABI)); err != nil {
		panic(err)
	}

	for name, gas := range map[string]uint64{
		OraclePriceMethodName:                15_000,
		OraclePricePairStateMethodName:       20_000,
		OraclePricePairStateScaledMethodName: 20_000,
	} {
		method, ok := oracleABI.Methods[name]
		if !ok {
			panic(name + " method not found in ABI")
		}
		var id [4]byte
		copy(id[:], method.ID[:4])
		oracleGasRequiredByMethod[id] = gas
	}
}

// pricePairStateTuple mirrors the Solidity PricePairState struct for ABI encoding and decoding.
// The json tags match the names used by go-ethereum's reflect.StructOf when building the
// anonymous tuple type, which is required for abi.ConvertType to work in tests and consumers.
type pricePairStateTuple struct {
	PairPrice            *big.Int `json:"pairPrice"`
	BasePrice            *big.Int `json:"basePrice"`
	QuotePrice           *big.Int `json:"quotePrice"`
	BaseCumulativePrice  *big.Int `json:"baseCumulativePrice"`
	QuoteCumulativePrice *big.Int `json:"quoteCumulativePrice"`
	BaseTimestamp        uint64   `json:"baseTimestamp"`
	QuoteTimestamp       uint64   `json:"quoteTimestamp"`
}

type Contract struct {
	oracleKeeper assistant.OracleKeeper
	kvGasConfig  storetypes.GasConfig
}

func NewContract(oracleKeeper assistant.OracleKeeper, kvGasConfig storetypes.GasConfig) vm.PrecompiledContract {
	return &Contract{
		oracleKeeper: oracleKeeper,
		kvGasConfig:  kvGasConfig,
	}
}

func (*Contract) ABI() abi.ABI {
	return oracleABI
}

func (*Contract) Address() common.Address {
	return oracleContractAddress
}

func (*Contract) Name() string {
	return "INJ_ORACLE"
}

func (c *Contract) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}

	baseCost := uint64(len(input)) * c.kvGasConfig.WriteCostPerByte
	var methodID [4]byte
	copy(methodID[:], input[:4])
	if methodGas, ok := oracleGasRequiredByMethod[methodID]; ok {
		return methodGas + baseCost
	}
	return baseCost
}

func (c *Contract) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	res, err := c.execute(evm, contract, readonly)
	if err != nil {
		return precomptypes.RevertReasonAndError(err)
	}
	return res, nil
}

func (c *Contract) execute(evm *vm.EVM, contract *vm.Contract, _ bool) (output []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errorsmod.Wrapf(ErrPrecompilePanic, "%v", r)
			output = nil
		}
	}()

	methodID := contract.Input[:4]
	method, err := oracleABI.MethodById(methodID)
	if err != nil {
		return nil, err
	}

	stateDB, ok := evm.StateDB.(precompiles.ExtStateDB)
	if !ok {
		return nil, errors.New("state DB does not implement ExtStateDB")
	}
	defer func(origCtx sdk.Context) { *stateDB.ContextPtr() = origCtx }(stateDB.Context())
	defer stateDB.Meter().FuncTiming(stateDB.ContextPtr(), "execute", metrics.Tag("svc", "oraclepc"), metrics.Tag("method", method.Name))()

	args, err := method.Inputs.Unpack(contract.Input[4:])
	if err != nil {
		return nil, errors.New("fail to unpack input arguments")
	}

	ctx := stateDB.CacheContext()
	if method.Name == OraclePriceMethodName {
		return c.oraclePrice(ctx, method, args)
	}
	if method.Name == OraclePricePairStateMethodName {
		return c.oraclePricePairState(ctx, method, args, nil)
	}
	return c.oraclePricePairStateScaled(ctx, method, args)
}

func (c *Contract) oraclePrice(ctx sdk.Context, m *abi.Method, args []any) ([]byte, error) {
	otU8, err := precomptypes.CastUint8(args[0])
	if err != nil {
		return nil, err
	}
	base, err := precomptypes.CastString(args[1])
	if err != nil {
		return nil, err
	}
	quote, err := precomptypes.CastString(args[2])
	if err != nil {
		return nil, err
	}

	oracleType := oracletypes.OracleType(otU8)
	a, err := assistant.NewOracleAssistant(c.oracleKeeper, oracleType)
	if err != nil {
		return nil, errorsmod.Wrapf(oracletypes.ErrOraclePriceNotFound,
			"type %s base %s quote %s", oracleType.String(), base, quote)
	}

	price := a.ReferencePrice(ctx, base, quote)
	if price == nil || price.IsNil() {
		return nil, errorsmod.Wrapf(oracletypes.ErrOraclePriceNotFound,
			"type %s base %s quote %s", oracleType.String(), base, quote)
	}

	return m.Outputs.Pack(price.BigInt())
}

func (c *Contract) oraclePricePairState(ctx sdk.Context, m *abi.Method, args []any, scaling *oracletypes.ScalingOptions) ([]byte, error) {
	pa, err := parsePairArgs(args)
	if err != nil {
		return nil, err
	}

	a, err := assistant.NewOracleAssistant(c.oracleKeeper, pa.oracleType)
	if err != nil {
		return nil, errorsmod.Wrapf(oracletypes.ErrOraclePriceNotFound,
			"type %s base %s quote %s", pa.oracleType.String(), pa.base, pa.quote)
	}

	pps := a.PricePairState(ctx, pa.base, pa.quote, scaling)
	if pps == nil {
		if scaling != nil {
			return nil, errorsmod.Wrapf(oracletypes.ErrOraclePriceNotFound,
				"type %s base %s quote %s baseDec %d quoteDec %d",
				pa.oracleType.String(), pa.base, pa.quote, scaling.BaseDecimals, scaling.QuoteDecimals)
		}
		return nil, errorsmod.Wrapf(oracletypes.ErrOraclePriceNotFound,
			"type %s base %s quote %s", pa.oracleType.String(), pa.base, pa.quote)
	}

	return m.Outputs.Pack(packPricePairState(pps))
}

func (c *Contract) oraclePricePairStateScaled(ctx sdk.Context, m *abi.Method, args []any) ([]byte, error) {
	baseDec, err := precomptypes.CastUint32(args[3])
	if err != nil {
		return nil, err
	}
	quoteDec, err := precomptypes.CastUint32(args[4])
	if err != nil {
		return nil, err
	}
	scaling := &oracletypes.ScalingOptions{BaseDecimals: baseDec, QuoteDecimals: quoteDec}
	return c.oraclePricePairState(ctx, m, args[:3], scaling)
}

type pairArgs struct {
	oracleType  oracletypes.OracleType
	base, quote string
}

// parsePairArgs extracts (oracleType, base, quote) from the first three ABI arguments.
func parsePairArgs(args []any) (pairArgs, error) {
	otU8, err := precomptypes.CastUint8(args[0])
	if err != nil {
		return pairArgs{}, err
	}
	base, err := precomptypes.CastString(args[1])
	if err != nil {
		return pairArgs{}, err
	}
	quote, err := precomptypes.CastString(args[2])
	if err != nil {
		return pairArgs{}, err
	}
	return pairArgs{oracleType: oracletypes.OracleType(otU8), base: base, quote: quote}, nil
}

// legacyDecToUint256 converts a LegacyDec to *big.Int, returning 0 for nil/zero-value decs.
// This prevents panics on USD-quote paths where quotePrice and quoteCumulativePrice are zero-value.
func legacyDecToUint256(d sdkmath.LegacyDec) *big.Int {
	if d.IsNil() {
		return new(big.Int)
	}
	return d.BigInt()
}

func packPricePairState(pps *oracletypes.PricePairState) pricePairStateTuple {
	return pricePairStateTuple{
		PairPrice:            legacyDecToUint256(pps.PairPrice),
		BasePrice:            legacyDecToUint256(pps.BasePrice),
		QuotePrice:           legacyDecToUint256(pps.QuotePrice),
		BaseCumulativePrice:  legacyDecToUint256(pps.BaseCumulativePrice),
		QuoteCumulativePrice: legacyDecToUint256(pps.QuoteCumulativePrice),
		BaseTimestamp:        safeTimestamp(pps.BaseTimestamp),
		QuoteTimestamp:       safeTimestamp(pps.QuoteTimestamp),
	}
}

func safeTimestamp(ts int64) uint64 {
	if ts < 0 {
		return 0
	}
	return uint64(ts)
}
