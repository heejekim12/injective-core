package testpeggy

import (
	"bytes"
	"context"
	"time"

	corestore "cosmossdk.io/core/store"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	ccrypto "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/peggy/types"
)

var (
	// Ensure that StakingKeeperMock implements required interface
	_ types.StakingKeeper = &StakingKeeperMock{}

	// ConsPrivKeys generate ed25519 ConsPrivKeys to be used for validator operator keys
	ConsPrivKeys = []ccrypto.PrivKey{
		ed25519.GenPrivKey(),
		ed25519.GenPrivKey(),
		ed25519.GenPrivKey(),
		ed25519.GenPrivKey(),
		ed25519.GenPrivKey(),
	}

	// ConsPubKeys holds the consensus public keys to be used for validator operator keys
	ConsPubKeys = []ccrypto.PubKey{
		ConsPrivKeys[0].PubKey(),
		ConsPrivKeys[1].PubKey(),
		ConsPrivKeys[2].PubKey(),
		ConsPrivKeys[3].PubKey(),
		ConsPrivKeys[4].PubKey(),
	}

	// AccPrivKeys generate secp256k1 pubkeys to be used for account pub keys
	AccPrivKeys = []ccrypto.PrivKey{
		secp256k1.GenPrivKey(),
		secp256k1.GenPrivKey(),
		secp256k1.GenPrivKey(),
		secp256k1.GenPrivKey(),
		secp256k1.GenPrivKey(),
	}

	// AccPubKeys holds the pub keys for the account keys
	AccPubKeys = []ccrypto.PubKey{
		AccPrivKeys[0].PubKey(),
		AccPrivKeys[1].PubKey(),
		AccPrivKeys[2].PubKey(),
		AccPrivKeys[3].PubKey(),
		AccPrivKeys[4].PubKey(),
	}

	// AccAddrs holds the sdk.AccAddresses
	AccAddrs = []sdk.AccAddress{
		sdk.AccAddress(AccPubKeys[0].Address()),
		sdk.AccAddress(AccPubKeys[1].Address()),
		sdk.AccAddress(AccPubKeys[2].Address()),
		sdk.AccAddress(AccPubKeys[3].Address()),
		sdk.AccAddress(AccPubKeys[4].Address()),
	}

	// ValAddrs holds the sdk.ValAddresses
	ValAddrs = []sdk.ValAddress{
		sdk.ValAddress(AccPubKeys[0].Address()),
		sdk.ValAddress(AccPubKeys[1].Address()),
		sdk.ValAddress(AccPubKeys[2].Address()),
		sdk.ValAddress(AccPubKeys[3].Address()),
		sdk.ValAddress(AccPubKeys[4].Address()),
	}

	// EthAddrs holds etheruem addresses
	EthAddrs = []common.Address{
		common.BytesToAddress(bytes.Repeat([]byte{byte(1)}, 20)),
		common.BytesToAddress(bytes.Repeat([]byte{byte(2)}, 20)),
		common.BytesToAddress(bytes.Repeat([]byte{byte(3)}, 20)),
		common.BytesToAddress(bytes.Repeat([]byte{byte(4)}, 20)),
		common.BytesToAddress(bytes.Repeat([]byte{byte(5)}, 20)),
	}

	// TokenContractAddrs holds example token contract addresses
	TokenContractAddrs = []string{
		common.HexToAddress("0x6b175474e89094c44da98b954eedeac495271d0f").Hex(), // DAI
		common.HexToAddress("0x0bc529c00c6401aef6d220be8c6ea1667f6ad93e").Hex(), // YFI
		common.HexToAddress("0x1f9840a85d5af5bf1d1762f925bdaddc4201f984").Hex(), // UNI
		common.HexToAddress("0xc00e94cb662c3520282e6f5717214004a7f26888").Hex(), // COMP
		common.HexToAddress("0xc011a73ee8576fb46f5e1c5751ca3b9fe0af2a6f").Hex(), // SNX
	}

	// InitTokens holds the number of tokens to initialize an account with
	InitTokens = sdk.TokensFromConsensusPower(110, sdk.DefaultPowerReduction)

	// InitCoins holds the number of coins to initialize an account with
	InitCoins = sdk.NewCoins(sdk.NewCoin(TestingStakeParams.BondDenom, InitTokens))

	// StakingAmount holds the staking power to start a validator with
	StakingAmount = sdk.TokensFromConsensusPower(10, sdk.DefaultPowerReduction)

	// StakingCoins holds the staking coins to start a validator with
	StakingCoins = sdk.NewCoins(sdk.NewCoin(TestingStakeParams.BondDenom, StakingAmount))

	// TestingStakeParams is a set of staking params for testing
	TestingStakeParams = stakingtypes.Params{
		UnbondingTime:     100,
		MaxValidators:     10,
		MaxEntries:        10,
		HistoricalEntries: 10000,
		BondDenom:         "stake",
		MinCommissionRate: math.LegacyZeroDec(),
	}

	// TestingPeggyParams is a set of peggy params for testing
	TestingPeggyParams = &types.Params{
		PeggyId:                       "testpeggyid",
		ContractSourceHash:            "62328f7bc12efb28f86111d08c29b39285680a906ea0e524e0209d6f6657b713",
		BridgeEthereumAddress:         common.HexToAddress("0x8858eeb3dfffa017d4bce9801d340d36cf895ccf").Hex(),
		CosmosCoinErc20Contract:       common.HexToAddress("0x8f3798462111bd6d7fa4d32ba0ab4ee4899bd4b7").Hex(),
		CosmosCoinDenom:               "inj",
		BridgeChainId:                 11,
		SignedBatchesWindow:           10,
		SignedValsetsWindow:           10,
		UnbondSlashingValsetsWindow:   15,
		SignedClaimsWindow:            10,
		TargetBatchTimeout:            60001,
		AverageBlockTime:              5000,
		AverageEthereumBlockTime:      15000,
		SlashFractionValset:           math.LegacyNewDecWithPrec(1, 2),
		SlashFractionBatch:            math.LegacyNewDecWithPrec(1, 2),
		SlashFractionClaim:            math.LegacyNewDecWithPrec(1, 2),
		SlashFractionConflictingClaim: math.LegacyNewDecWithPrec(1, 2),
		SlashFractionBadEthSignature:  math.LegacyNewDecWithPrec(1, 2),
	}
)

func GetDefaultValidatorSet() []ValidatorInfo {
	return []ValidatorInfo{
		{
			AccAddr:  AccAddrs[0],
			OrchAddr: AccAddrs[0],
			ValAddr:  ValAddrs[0],
			EthAddr:  EthAddrs[0],
			ConsKey:  ConsPubKeys[0],
			PubKey:   AccPubKeys[0],
		},

		{
			AccAddr:  AccAddrs[1],
			OrchAddr: AccAddrs[1],
			ValAddr:  ValAddrs[1],
			EthAddr:  EthAddrs[1],
			ConsKey:  ConsPubKeys[1],
			PubKey:   AccPubKeys[1],
		},

		{
			AccAddr:  AccAddrs[2],
			OrchAddr: AccAddrs[2],
			ValAddr:  ValAddrs[2],
			EthAddr:  EthAddrs[2],
			ConsKey:  ConsPubKeys[2],
			PubKey:   AccPubKeys[2],
		},

		{
			AccAddr:  AccAddrs[3],
			OrchAddr: AccAddrs[3],
			ValAddr:  ValAddrs[3],
			EthAddr:  EthAddrs[3],
			ConsKey:  ConsPubKeys[3],
			PubKey:   AccPubKeys[3],
		},

		{
			AccAddr:  AccAddrs[4],
			OrchAddr: AccAddrs[4],
			ValAddr:  ValAddrs[4],
			EthAddr:  EthAddrs[4],
			ConsKey:  ConsPubKeys[4],
			PubKey:   AccPubKeys[4],
		},
	}
}

type ValidatorInfo struct {
	AccAddr  sdk.AccAddress
	OrchAddr sdk.AccAddress
	ValAddr  sdk.ValAddress
	EthAddr  common.Address
	ConsKey,
	PubKey ccrypto.PubKey
	PrivKey ccrypto.PrivKey
}

func GenerateNewValidatorInfo() ValidatorInfo {
	privKey := secp256k1.GenPrivKey()

	return ValidatorInfo{
		AccAddr: sdk.AccAddress(privKey.PubKey().Address()),
		ValAddr: sdk.ValAddress(privKey.PubKey().Address()),
		ConsKey: ed25519.GenPrivKey().PubKey(),
		PubKey:  privKey.PubKey(),
		EthAddr: common.BytesToAddress(privKey.PubKey().Bytes()),
	}
}

// nolint:all
// MintVouchersFromAir creates new peggy vouchers given erc20tokens
//func MintVouchersFromAir(t *testing.T, ctx sdk.Context, k peggyKeeper.Keeper, dest sdk.AccAddress, amount types.ERC20Token) sdk.Coin {
//	coin := amount.PeggyCoin()
//	vouchers := sdk.Coins{coin}
//	err := k.BankKeeper.MintCoins(ctx, types.ModuleName, vouchers)
//	err = k.BankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, dest, vouchers)
//	require.NoError(t, err)
//	return coin
//}

// NewStakingKeeperMock creates a new mock staking keeper
func NewStakingKeeperMock(operators ...sdk.ValAddress) *StakingKeeperMock {
	r := &StakingKeeperMock{
		BondedValidators: make([]stakingtypes.Validator, 0),
		ValidatorPower:   make(map[string]int64),
	}
	const defaultTestPower = 100
	for _, a := range operators {
		r.BondedValidators = append(r.BondedValidators, stakingtypes.Validator{
			OperatorAddress: a.String(),
			Status:          stakingtypes.Bonded,
		})
		r.ValidatorPower[a.String()] = defaultTestPower
	}
	return r
}

// MockStakingValidatorData creates mock validator data
type MockStakingValidatorData struct {
	Operator sdk.ValAddress
	Power    int64
}

// NewStakingKeeperWeightedMock creates a new mock staking keeper with some mock validator data
func NewStakingKeeperWeightedMock(t ...MockStakingValidatorData) *StakingKeeperMock {
	r := &StakingKeeperMock{
		BondedValidators: make([]stakingtypes.Validator, len(t)),
		ValidatorPower:   make(map[string]int64, len(t)),
	}

	for i, a := range t {
		r.BondedValidators[i] = stakingtypes.Validator{
			OperatorAddress: a.Operator.String(),
			Status:          stakingtypes.Bonded,
		}
		r.ValidatorPower[a.Operator.String()] = a.Power
	}
	return r
}

// StakingKeeperMock is a mock staking keeper for use in the tests
type StakingKeeperMock struct {
	BondedValidators []stakingtypes.Validator
	ValidatorPower   map[string]int64
}

func (s *StakingKeeperMock) PowerReduction(ctx context.Context) (res math.Int) {
	return sdk.DefaultPowerReduction
}

// GetBondedValidatorsByPower implements the interface for staking keeper required by peggy
func (s *StakingKeeperMock) GetBondedValidatorsByPower(ctx context.Context) ([]stakingtypes.Validator, error) {
	return s.BondedValidators, nil
}

// GetLastValidatorPower implements the interface for staking keeper required by peggy
func (s *StakingKeeperMock) GetLastValidatorPower(ctx context.Context, operator sdk.ValAddress) (int64, error) {
	v, ok := s.ValidatorPower[operator.String()]
	if !ok {
		panic("unknown address")
	}
	return v, nil
}

// GetLastTotalPower implements the interface for staking keeper required by peggy
func (s *StakingKeeperMock) GetLastTotalPower(ctx context.Context) (math.Int, error) {
	var total int64
	for _, v := range s.ValidatorPower {
		total += v
	}
	return math.NewInt(total), nil
}

// IterateValidators staisfies the interface
func (s *StakingKeeperMock) IterateValidators(ctx context.Context, cb func(index int64, validator stakingtypes.ValidatorI) (stop bool)) error {
	validators := s.BondedValidators
	for i := range s.BondedValidators {
		stop := cb(int64(i), validators[i])
		if stop {
			break
		}
	}
	return nil
}

// IterateBondedValidatorsByPower staisfies the interface
func (s *StakingKeeperMock) IterateBondedValidatorsByPower(ctx context.Context, cb func(index int64, validator stakingtypes.ValidatorI) (stop bool)) error {
	validators := s.BondedValidators
	for i := range validators {
		stop := cb(int64(i), validators[i])
		if stop {
			break
		}
	}
	return nil
}

// IterateLastValidators staisfies the interface
func (s *StakingKeeperMock) IterateLastValidators(ctx context.Context, cb func(index int64, validator stakingtypes.ValidatorI) (stop bool)) error {
	validators := s.BondedValidators
	for i := range s.BondedValidators {
		stop := cb(int64(i), validators[i])
		if stop {
			break
		}
	}
	return nil
}

// Validator staisfies the interface
func (s *StakingKeeperMock) Validator(cont context.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error) {
	var err error
	validators := s.BondedValidators
	for i := range s.BondedValidators {
		valAddr, err := sdk.ValAddressFromBech32(validators[i].GetOperator())
		if err == nil && valAddr.Equals(addr) {
			return validators[i], err
		}
	}
	return nil, err
}

// ValidatorByConsAddr staisfies the interface
func (s *StakingKeeperMock) ValidatorByConsAddr(ctx context.Context, addr sdk.ConsAddress) (stakingtypes.ValidatorI, error) {
	var err error
	validators := s.BondedValidators
	for i := range s.BondedValidators {
		cons, err := validators[i].GetConsAddr()
		if err != nil {
			panic(err)
		}
		consAddr, err := sdk.ConsAddressFromBech32(string(cons))
		if consAddr.Equals(addr) {
			return validators[i], nil
		}
	}
	return nil, err
}

func (s *StakingKeeperMock) GetParams(ctx context.Context) (stakingtypes.Params, error) {
	panic("unexpected call")
}

func (s *StakingKeeperMock) GetValidator(ctx context.Context, addr sdk.ValAddress) (validator stakingtypes.Validator, err error) {
	panic("unexpected call")
}

func (s *StakingKeeperMock) ValidatorQueueIterator(ctx context.Context, endTime time.Time, endHeight int64) (corestore.Iterator, error) {
	panic("unexpected call")
}

// Slash staisfies the interface
func (s *StakingKeeperMock) Slash(context.Context, sdk.ConsAddress, int64, int64, math.LegacyDec) (math.Int, error) {
	return math.Int{}, nil
}

// Jail staisfies the interface
func (s *StakingKeeperMock) Jail(context.Context, sdk.ConsAddress) error {
	return nil
}

// AlwaysPanicStakingMock is a mock staking keeper that panics on usage
type AlwaysPanicStakingMock struct{}

// GetLastTotalPower implements the interface for staking keeper required by peggy
func (s AlwaysPanicStakingMock) GetLastTotalPower(ctx sdk.Context) (power math.Int) {
	panic("unexpected call")
}

// GetBondedValidatorsByPower implements the interface for staking keeper required by peggy
func (s AlwaysPanicStakingMock) GetBondedValidatorsByPower(ctx sdk.Context) []stakingtypes.Validator {
	panic("unexpected call")
}

// GetLastValidatorPower implements the interface for staking keeper required by peggy
func (s AlwaysPanicStakingMock) GetLastValidatorPower(ctx sdk.Context, operator sdk.ValAddress) int64 {
	panic("unexpected call")
}

// IterateValidators staisfies the interface
func (s AlwaysPanicStakingMock) IterateValidators(sdk.Context, func(index int64, validator stakingtypes.ValidatorI) (stop bool)) {
	panic("unexpected call")
}

// IterateBondedValidatorsByPower staisfies the interface
func (s AlwaysPanicStakingMock) IterateBondedValidatorsByPower(sdk.Context, func(index int64, validator stakingtypes.ValidatorI) (stop bool)) {
	panic("unexpected call")
}

// IterateLastValidators staisfies the interface
func (s AlwaysPanicStakingMock) IterateLastValidators(sdk.Context, func(index int64, validator stakingtypes.ValidatorI) (stop bool)) {
	panic("unexpected call")
}

// Validator staisfies the interface
func (s AlwaysPanicStakingMock) Validator(sdk.Context, sdk.ValAddress) stakingtypes.ValidatorI {
	panic("unexpected call")
}

// ValidatorByConsAddr staisfies the interface
func (s AlwaysPanicStakingMock) ValidatorByConsAddr(sdk.Context, sdk.ConsAddress) stakingtypes.ValidatorI {
	panic("unexpected call")
}

// Slash staisfies the interface
func (s AlwaysPanicStakingMock) Slash(sdk.Context, sdk.ConsAddress, int64, int64, math.LegacyDec) {
	panic("unexpected call")
}

// Jail staisfies the interface
func (s AlwaysPanicStakingMock) Jail(sdk.Context, sdk.ConsAddress) {
	panic("unexpected call")
}

func NewTestMsgCreateValidator(address sdk.ValAddress, pubKey ccrypto.PubKey, amt math.Int) *stakingtypes.MsgCreateValidator {
	commission := stakingtypes.NewCommissionRates(
		math.LegacyMustNewDecFromStr("0.05"),
		math.LegacyMustNewDecFromStr("0.05"),
		math.LegacyMustNewDecFromStr("0.05"),
	)

	out, err := stakingtypes.NewMsgCreateValidator(
		address.String(),
		pubKey,
		sdk.NewCoin("stake", amt),
		stakingtypes.Description{Moniker: "some moniker"},
		commission,
		math.OneInt(),
	)
	if err != nil {
		panic(err)
	}

	return out
}

func NewTestMsgUnDelegateValidator(address sdk.ValAddress, amt math.Int) *stakingtypes.MsgUndelegate {
	msg := stakingtypes.NewMsgUndelegate(sdk.AccAddress(address).String(), address.String(), sdk.NewCoin("stake", amt))
	return msg
}
