package helpers

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/InjectiveLabs/etherman/deployer"
	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	tokenfactorytypes "github.com/InjectiveLabs/sdk-go/chain/tokenfactory/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	transfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/strangelove-ventures/interchaintest/v8"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/chain/ethereum"
	"github.com/strangelove-ventures/interchaintest/v8/chain/ethereum/geth"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PeggyContractName            = "Peggy"
	ProxyAdminContractName       = "ProxyAdmin"
	TransparentProxyContractName = "TransparentUpgradeableProxy"
	InjERC20ContractName         = "InjERC20"

	PeggyContractPath            = "../peggo/solidity/contracts/Peggy.sol"
	ProxyAdminContractPath       = "../peggo/solidity/contracts/@openzeppelin/contracts/ProxyAdmin.sol"
	TransparentProxyContractPath = "../peggo/solidity/contracts/@openzeppelin/contracts/TransparentUpgradeableProxy.sol"
	InjERC20ContractPath         = "../peggo/solidity/contracts/InjToken.sol"
)

type PeggyContractSuite struct {
	Peggy            common.Address
	ProxyAdmin       common.Address
	TransparentProxy common.Address
	InjectiveCoin    common.Address
	StartHeight      uint64
}

// GetCurrentValset returns the current validator set on Injective using gRPC.
func GetCurrentValset(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
) *peggytypes.Valset {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := peggytypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.CurrentValset, &peggytypes.QueryCurrentValsetRequest{})
	require.NoError(t, err, "error querying current valset")

	return resp.Valset
}

func RegisterOrchestrator(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	node *cosmos.ChainNode,
	orchestratorAddress,
	ethereumAddress string,
) {
	t.Helper()

	txHash, err := BroadcastMsgWithKeyringAsync(ctx, chain, node, ValidatorKeyName, DefaultGas, func(validatorAddr sdktypes.AccAddress) ([]sdktypes.Msg, error) {
		return []sdktypes.Msg{
			&peggytypes.MsgSetOrchestratorAddresses{
				Sender:       validatorAddr.String(),
				Orchestrator: orchestratorAddress,
				EthAddress:   ethereumAddress,
			},
		}, nil
	})
	require.NoError(t, err, "failed to broadcast register orchestrator tx")

	txResp, err := QueryTxRPC(ctx, node, txHash)
	require.NoError(t, err, "failed to wait for register orchestrator tx")
	require.Equal(t, uint32(0), txResp.ErrorCode, "register orchestrator tx failed")

	err = testutil.WaitForBlocks(ctx, 1, chain)
	require.NoError(t, err)

	t.Log("registered orchestrator",
		"orchestrator_address="+orchestratorAddress,
		"eth_address="+ethereumAddress,
	)
}

func GetValidatorPrivateKey(
	t *testing.T,
	ctx context.Context,
	node *cosmos.ChainNode,
) string {
	t.Helper()

	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf(`echo -e "12345678\n12345678" | injectived keys unsafe-export-eth-key validator --home %s --keyring-backend %s`, node.HomeDir(), keyring.BackendTest),
	}

	stdout, _, err := node.Exec(ctx, cmd, node.Chain.Config().Env)
	require.NoError(t, err)

	return strings.TrimSpace(string(stdout))
}

func DeployPeggyContractSuite(
	t *testing.T,
	ctx context.Context,
	chain *geth.GethChain,
	vs *peggytypes.Valset,
) PeggyContractSuite {
	t.Helper()

	contractDeployerMnemonic := "pony glide frown crisp unfold lawn cup loan trial govern usual matrix theory wash fresh address pioneer between meadow visa buffalo keep gallery swear"

	deriveFn := hd.Secp256k1.Derive()
	pk, err := deriveFn(contractDeployerMnemonic, "", hd.CreateHDPath(60, 0, 0).String())
	require.NoError(t, err)
	contractDeployerPK := hd.Secp256k1.Generate()(pk)
	_ = contractDeployerPK

	contractDeployer, err := chain.BuildWallet(ctx, "deployer", contractDeployerMnemonic)
	require.NoError(t, err)

	chainCfg := chain.Config()
	ethUserInitialAmount := ethereum.ETHER.MulRaw(1000)

	err = chain.SendFunds(ctx, interchaintest.FaucetAccountKeyName, ibc.WalletAmount{
		Address: contractDeployer.FormattedAddress(),
		Amount:  ethUserInitialAmount,
		Denom:   chainCfg.Denom,
	})
	require.NoError(t, err)

	d, err := deployer.New(
		deployer.OptionEVMRPCEndpoint(chain.GetHostRPCAddress()),
		deployer.OptionGasLimit(10000000),
		deployer.OptionRPCTimeout(30*time.Second),
		deployer.OptionTxTimeout(30*time.Second),
		deployer.OptionCallTimeout(30*time.Second),
		deployer.OptionGasPrice(big.NewInt(3000000000)),
	)
	require.NoError(t, err)

	ecdsaPK, err := crypto.HexToECDSA(hex.EncodeToString(contractDeployerPK.Bytes()))
	require.NoError(t, err)

	peggyDeployOpts := deployer.ContractDeployOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    PeggyContractPath,
		ContractName: PeggyContractName,
		Await:        true,
	}

	_, peggyContract, err := d.Deploy(ctx, peggyDeployOpts, func(args abi.Arguments) []interface{} {
		return nil
	})
	require.NoError(t, err)

	var (
		peggyID    = common.HexToHash("0x696e6a6563746976652d70656767796964000000000000000000000000000000")
		minPower   *big.Int
		validators []common.Address
		powers     []*big.Int
	)

	totalPower := big.NewInt(0)
	for _, member := range vs.Members {
		totalPower = totalPower.Add(totalPower, big.NewInt(0).SetUint64(member.Power))
	}

	minPower = big.NewInt(0).Mul(totalPower, big.NewInt(2))
	minPower = minPower.Quo(minPower, big.NewInt(3))

	for _, member := range vs.Members {
		validators = append(validators, common.HexToAddress(member.EthereumAddress))
		powers = append(powers, big.NewInt(0).SetUint64(member.Power))
	}

	deployArgs := []any{
		peggyID,
		minPower,
		validators,
		powers,
	}

	peggyTxOpts := deployer.ContractTxOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    PeggyContractPath,
		ContractName: PeggyContractName,
		Contract:     peggyContract.Address,
		Await:        true,
		BytecodeOnly: true,
	}

	contractStartHeight, err := chain.Height(ctx)
	require.NoError(t, err)

	_, initData, err := d.Tx(ctx, peggyTxOpts, "initialize", func(_ abi.Arguments) []interface{} {
		return deployArgs
	})
	require.NoError(t, err)
	require.NotNil(t, initData)

	t.Log("deployed Peggy.sol", peggyContract.Address.String())
	time.Sleep(1 * time.Second)

	proxyAdminOpts := deployer.ContractDeployOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    ProxyAdminContractPath,
		ContractName: ProxyAdminContractName,
		Await:        true,
	}

	_, proxyAdminContract, err := d.Deploy(ctx, proxyAdminOpts, func(args abi.Arguments) []interface{} {
		return nil
	})
	require.NoError(t, err)

	t.Log("deployed ProxyAdmin.sol", proxyAdminContract.Address.String())
	time.Sleep(1 * time.Second)

	transparentProxyOpts := deployer.ContractDeployOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    TransparentProxyContractPath,
		ContractName: TransparentProxyContractName,
		Await:        true,
	}

	proxyArgs := []any{
		peggyContract.Address,
		proxyAdminContract.Address,
		initData,
	}

	_, transparentProxyContract, err := d.Deploy(ctx, transparentProxyOpts, func(args abi.Arguments) []interface{} {
		return proxyArgs
	})
	require.NoError(t, err)

	t.Log("deployed TransparentUpgradeableProxy.sol", transparentProxyContract.Address.String())
	time.Sleep(1 * time.Second)

	injectiveCoinOpts := deployer.ContractDeployOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    InjERC20ContractPath,
		ContractName: InjERC20ContractName,
		Await:        true,
	}

	injectiveCoinArgs := []any{
		"Injective",
		"INJ",
		uint8(18),
	}

	_, injectiveCoinContract, err := d.Deploy(ctx, injectiveCoinOpts, func(args abi.Arguments) []interface{} {
		return injectiveCoinArgs
	})
	require.NoError(t, err)

	t.Log("deployed Injective Token (CosmosToken.sol)", injectiveCoinContract.Address.String())

	mintOpts := deployer.ContractTxOpts{
		From:         common.HexToAddress(contractDeployer.FormattedAddress()),
		FromPk:       ecdsaPK,
		SolSource:    InjERC20ContractPath,
		ContractName: InjERC20ContractName,
		Contract:     injectiveCoinContract.Address,
		Await:        true,
	}

	mintArgs := []any{
		transparentProxyContract.Address,
		math.NewIntWithDecimal(100_000_000, 18).BigInt(), // 100M INJ
	}

	_, _, err = d.Tx(ctx, mintOpts, "mint", func(args abi.Arguments) []interface{} {
		return mintArgs
	})
	require.NoError(t, err, "failed to mint Injective token to Proxy")

	t.Log("minted 100M INJ tokens to Peggy Proxy")

	return PeggyContractSuite{
		Peggy:            peggyContract.Address,
		ProxyAdmin:       proxyAdminContract.Address,
		TransparentProxy: transparentProxyContract.Address,
		InjectiveCoin:    injectiveCoinContract.Address,
		StartHeight:      uint64(contractStartHeight),
	}
}

func UpdatePeggyParams(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	params *peggytypes.Params,
) {
	t.Helper()

	msg := &peggytypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    *params,
	}

	funds := math.NewIntWithDecimal(1_000_000, 18)
	proposer, err := interchaintest.GetAndFundTestUserWithMnemonic(ctx, t.Name(), NewMnemonic(), funds, chain)
	require.NoError(t, err)

	MustSucceedProposal(t, chain, ctx, proposer, msg, "Update Peggy module Params")

	t.Log("peggy params updated")
}

func GetPeggoEnvDefaults(
	injectiveChain *cosmos.CosmosChain,
	gethChain *geth.GethChain,
	cosmosPK string,
	ethPK string,
	transparentProxyContract common.Address,
) []string {
	return []string{
		"PEGGO_ENV=local",
		"PEGGO_LOG_LEVEL=debug",
		"PEGGO_SERVICE_WAIT_TIMEOUT=1m",
		"PEGGO_COSMOS_CHAIN_ID=" + injectiveChain.Config().ChainID,
		"PEGGO_COSMOS_GRPC=" + injectiveChain.GetGRPCAddress(),
		"PEGGO_TENDERMINT_RPC=" + injectiveChain.GetRPCAddress(),
		"PEGGO_COSMOS_FEE_DENOM=inj",
		"PEGGO_COSMOS_GAS_PRICES=" + injectiveChain.Config().GasPrices,
		"PEGGO_COSMOS_PK=" + cosmosPK,
		"PEGGO_COSMOS_USE_LEDGER=false",
		"PEGGO_ETH_CHAIN_ID=" + gethChain.Config().ChainID,
		"PEGGO_ETH_RPC=" + gethChain.GetRPCAddress(),
		"PEGGO_ETH_CONTRACT_ADDRESS=" + transparentProxyContract.String(),
		"PEGGO_COINGECKO_API=https://api.coingecko.com/api/v3",
		"PEGGO_ETH_PK=" + ethPK,
		"PEGGO_ETH_USE_LEDGER=false",
		"PEGGO_ETH_GAS_PRICE_ADJUSTMENT=1.3",
		"PEGGO_ETH_MAX_GAS_PRICE=500gwei",
		"PEGGO_RELAY_VALSETS=true",
		"PEGGO_RELAY_VALSET_OFFSET_DUR=0m", // test speed
		"PEGGO_RELAY_BATCHES=true",
		"PEGGO_RELAY_BATCH_OFFSET_DUR=0m", // test speed
		"PEGGO_RELAY_PENDING_TX_WAIT_DURATION=20m",
		"PEGGO_MIN_BATCH_FEE_USD=0", // this must be set to 0 otherwise peggo will query coingecko for token price
		"PEGGO_STATSD_PREFIX=peggo.",
		"PEGGO_STATSD_ADDR=localhost:8125",
		"PEGGO_STATSD_STUCK_DUR=5m",
		"PEGGO_STATSD_MOCKING=false",
		"PEGGO_STATSD_DISABLED=true",
		"PEGGO_HEALTH_CHECK_PORT=7070",
		// shorten test time
		"PEGGO_LOOP_DURATION=5s",
		"PEGGO_RELAYER_LOOP_DURATION=10s",
		"PEGGO_RELAY_VALSET_OFFSET_DUR=0m",
		"PEGGO_RELAY_BATCH_OFFSET_DUR=0m",
	}
}

func AwaitLastObservedValsetNonce(
	t *testing.T,
	ctx context.Context,
	dur time.Duration,
	chain *cosmos.CosmosChain,
	valsetNonce uint64,
) {
	t.Helper()

	state := GetPeggyModuleState(t, ctx, chain)
	if state == nil {
		panic("nil state")
	}

	if valsetNonce <= state.LastObservedValset.Nonce {
		return
	}

	timeout := time.After(dur)
	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			t.Fatal("timed out waiting for last_observed_valset nonce:",
				"expected:", valsetNonce,
				"actual:", state.LastObservedValset.Nonce,
			)
		case <-ticker.C:
			state = GetPeggyModuleState(t, ctx, chain)

			if valsetNonce <= state.LastObservedValset.Nonce {
				t.Log("last_observed_valset nonce:", state.LastObservedValset.Nonce)
				return
			}

			t.Log("waiting for last_observed_valset nonce:",
				"expected:", valsetNonce,
				"actual:", state.LastObservedValset.Nonce,
			)
		}
	}
}

func GetPeggyModuleState(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
) *peggytypes.GenesisState {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := peggytypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.PeggyModuleState, &peggytypes.QueryModuleStateRequest{})
	require.NoError(t, err, "error querying peggy module state")

	return resp.State
}

type PeggyRateLimitTransfersJSON struct {
	Token    string            `json:"token"`
	Inflows  []json.RawMessage `json:"inflows"`
	Outflows []json.RawMessage `json:"outflows"`
}

// todo(dusan): this goes away after sdk-go includes updated peggy proto
func GetPeggyRateLimitTransfersJSON(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
) []*PeggyRateLimitTransfersJSON {
	t.Helper()

	stdout, _, err := chain.GetFullNode().ExecQuery(ctx, "peggy", "module-state", "--chain-id", chain.Config().ChainID)
	require.NoError(t, err, "error querying peggy module state via CLI")

	var resp struct {
		State struct {
			RateLimitTransfers []*PeggyRateLimitTransfersJSON `json:"rate_limit_transfers"`
		} `json:"state"`
	}

	err = json.Unmarshal(stdout, &resp)
	require.NoError(t, err, "error decoding peggy module state JSON")

	return resp.State.RateLimitTransfers
}

func SendToInjective(
	t *testing.T,
	ctx context.Context,
	chain *geth.GethChain,
	senderPK *ecdsa.PrivateKey,
	receiver ibc.Wallet,
	amount *big.Int,
	contracts PeggyContractSuite,
) {
	t.Helper()

	d, err := deployer.New(
		deployer.OptionEVMRPCEndpoint(chain.GetHostRPCAddress()),
		deployer.OptionGasLimit(10000000),
		deployer.OptionRPCTimeout(30*time.Second),
		deployer.OptionTxTimeout(30*time.Second),
		deployer.OptionCallTimeout(30*time.Second),
		deployer.OptionGasPrice(big.NewInt(3000000000)),
	)
	require.NoError(t, err)

	opts := deployer.ContractTxOpts{
		From:         crypto.PubkeyToAddress(senderPK.PublicKey),
		FromPk:       senderPK,
		SolSource:    InjERC20ContractPath,
		ContractName: InjERC20ContractName,
		Contract:     contracts.InjectiveCoin,
		Await:        true,
	}

	args := []any{
		contracts.TransparentProxy,
		amount,
	}

	_, _, err = d.Tx(ctx, opts, "approve", func(_ abi.Arguments) []interface{} {
		return args
	})
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	receiverBz := PrependZeroBytes12(receiver.Address())

	var receiver32 [32]byte
	copy(receiver32[:], receiverBz)

	args = []any{
		contracts.InjectiveCoin,
		receiver32,
		amount,
		"",
	}

	opts = deployer.ContractTxOpts{
		From:         crypto.PubkeyToAddress(senderPK.PublicKey),
		FromPk:       senderPK,
		SolSource:    PeggyContractPath,
		ContractName: PeggyContractName,
		Contract:     contracts.TransparentProxy,
		Await:        true,
	}

	_, _, err = d.Tx(ctx, opts, "sendToInjective", func(_ abi.Arguments) []interface{} {
		return args
	})
	require.NoError(t, err)
}

func PrependZeroBytes12(data []byte) []byte {
	return append(make([]byte, 12), data...)
}

func SendToEth(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	sender ibc.Wallet,
	receiver ibc.Wallet,
	coin sdktypes.Coin,
	fee sdktypes.Coin,
) uint64 {
	t.Helper()

	msg := &peggytypes.MsgSendToEth{
		Sender:    sender.FormattedAddress(),
		EthDest:   receiver.FormattedAddress(),
		Amount:    coin,
		BridgeFee: fee,
	}

	resp := BroadcastTxBlock(t, ctx, chain, sender, []cosmos.FactoryOpt{WithGas(DefaultGas)}, msg)
	require.EqualValues(t, uint32(0), resp.Code, "send to eth tx failed: %s", resp.RawLog)

	return GetPeggyModuleState(t, ctx, chain).LastOutgoingPoolId
}

func GetIBCDenomTraces(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
) *transfertypes.QueryDenomTracesResponse {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := transfertypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.DenomTraces, &transfertypes.QueryDenomTracesRequest{})
	require.NoError(t, err, "error querying IBC denom traces")

	return resp
}

func SetDenomMetadata(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	metadata *banktypes.Metadata,
) {
	t.Helper()

	msg := &tokenfactorytypes.MsgSetDenomMetadata{
		Sender:   authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Metadata: *metadata,
	}

	funds := math.NewIntWithDecimal(1_000_000, 18)
	proposer, err := interchaintest.GetAndFundTestUserWithMnemonic(ctx, t.Name(), NewMnemonic(), funds, chain)
	require.NoError(t, err)

	MustSucceedProposal(t, chain, ctx, proposer, msg, "Update IBC denom metadata")
}

func DeployERC20(
	t *testing.T,
	ctx context.Context,
	chain *geth.GethChain,
	denomMetadata *banktypes.Metadata,
	peggyContract common.Address,
) {
	t.Helper()

	deployerKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainCfg := chain.Config()
	ethUserInitialAmount := ethereum.ETHER.MulRaw(1000)
	deployerFunds := ibc.WalletAmount{
		Address: crypto.PubkeyToAddress(deployerKey.PublicKey).String(),
		Amount:  ethUserInitialAmount,
		Denom:   chainCfg.Denom,
	}

	require.NoError(t, chain.SendFunds(ctx, interchaintest.FaucetAccountKeyName, deployerFunds))

	d, err := deployer.New(
		deployer.OptionEVMRPCEndpoint(chain.GetHostRPCAddress()),
		deployer.OptionGasLimit(10000000),
		deployer.OptionRPCTimeout(30*time.Second),
		deployer.OptionTxTimeout(30*time.Second),
		deployer.OptionCallTimeout(30*time.Second),
		deployer.OptionGasPrice(big.NewInt(3000000000)),
	)
	require.NoError(t, err)

	opts := deployer.ContractTxOpts{
		From:         crypto.PubkeyToAddress(deployerKey.PublicKey),
		FromPk:       deployerKey,
		SolSource:    PeggyContractPath,
		ContractName: PeggyContractName,
		Contract:     peggyContract,
		Await:        true,
	}

	args := []any{
		denomMetadata.Base,
		denomMetadata.Display,
		denomMetadata.Display,
		uint8(denomMetadata.Decimals),
	}

	_, _, err = d.Tx(ctx, opts, "deployERC20", func(_ abi.Arguments) []interface{} {
		return args
	})
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	t.Log("deployed ERC20 on Peggy.sol", "base:", denomMetadata.Base, "display:", denomMetadata.Display)
}

func AwaitLastObservedEventNonce(
	t *testing.T,
	ctx context.Context,
	dur time.Duration,
	chain *cosmos.CosmosChain,
	nonce uint64,
) {
	t.Helper()

	isEventNonceObserved := func(stateNonce uint64) bool {
		return nonce <= stateNonce
	}

	state := GetPeggyModuleState(t, ctx, chain)
	if isEventNonceObserved(state.LastObservedNonce) {
		return
	}

	timeout := time.After(dur)
	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			t.Fatal("timed out waiting for update of last_observed_nonce:",
				"expected:", nonce,
				"observed:", state.LastObservedNonce,
			)
		case <-ticker.C:
			state = GetPeggyModuleState(t, ctx, chain)
			if isEventNonceObserved(state.LastObservedNonce) {
				t.Log("last_observed_nonce:", state.LastObservedNonce)
				return
			}

			t.Log("waiting for update of last_observed_nonce:",
				"expected:", nonce,
				"observed:", state.LastObservedNonce,
			)
		}
	}
}

func AwaitLastOutgoingBatchID(
	t *testing.T,
	ctx context.Context,
	dur time.Duration,
	chain *cosmos.CosmosChain,
	batchNonce uint64,
) {
	t.Helper()

	isBatchAlreadyCreated := func(stateNonce uint64) bool {
		return batchNonce <= stateNonce
	}

	latestBatchID := GetPeggyModuleState(t, ctx, chain).LastOutgoingBatchId
	if isBatchAlreadyCreated(latestBatchID) {
		return
	}

	timeout := time.After(dur)
	ticker := time.NewTicker(3 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			t.Fatal("timed out waiting for update of last_outgoing_batch_id:",
				"expected:", batchNonce,
				"observed:", latestBatchID,
			)
		case <-ticker.C:
			latestBatchID = GetPeggyModuleState(t, ctx, chain).LastOutgoingBatchId
			if isBatchAlreadyCreated(latestBatchID) {
				t.Log("last_outgoing_batch_id:", latestBatchID)
				return
			}

			t.Log("waiting for update of last_outgoing_batch_id:",
				"expected:", batchNonce,
				"observed:", latestBatchID,
			)
		}
	}
}

func ParseWithdrawClaim(
	t *testing.T,
	chain *cosmos.CosmosChain,
	att *peggytypes.Attestation,
) *peggytypes.MsgWithdrawClaim {
	t.Helper()

	var claim peggytypes.EthereumClaim
	require.NoError(t, chain.Config().EncodingConfig.Codec.UnpackAny(att.Claim, &claim))
	require.Equal(t, peggytypes.CLAIM_TYPE_WITHDRAW.String(), claim.GetType().String())

	return claim.(*peggytypes.MsgWithdrawClaim)
}

func ParseDepositClaim(
	t *testing.T,
	chain *cosmos.CosmosChain,
	att *peggytypes.Attestation,
) *peggytypes.MsgDepositClaim {
	t.Helper()

	var claim peggytypes.EthereumClaim
	require.NoError(t, chain.Config().EncodingConfig.Codec.UnpackAny(att.Claim, &claim))
	require.Equal(t, peggytypes.CLAIM_TYPE_DEPOSIT.String(), claim.GetType().String())

	return claim.(*peggytypes.MsgDepositClaim)
}

func ParseERC20DeployedClaim(
	t *testing.T,
	chain *cosmos.CosmosChain,
	att *peggytypes.Attestation,
) *peggytypes.MsgERC20DeployedClaim {
	t.Helper()

	var claim peggytypes.EthereumClaim
	require.NoError(t, chain.Config().EncodingConfig.Codec.UnpackAny(att.Claim, &claim))
	require.Equal(t, peggytypes.CLAIM_TYPE_ERC20_DEPLOYED.String(), claim.GetType().String())

	return claim.(*peggytypes.MsgERC20DeployedClaim)
}

type OrchestratorStatus struct {
	LastObservedEventNonceByNetwork      uint64   `json:"last_observed_event_nonce_by_network"`
	LastObservedEventNonceByOrchestrator uint64   `json:"last_observed_event_nonce_by_orchestrator"`
	PendingTxBatchToSign                 uint64   `json:"pending_tx_batch_to_sign"`
	PendingValidatorSetsToSign           []uint64 `json:"pending_validator_sets_to_sign"`
	IsPartOfTheCurrentSet                bool     `json:"is_part_of_the_current_set"`
}

func PeggoHealthCheck(t *testing.T, ctx context.Context, node *cosmos.ChainNode) OrchestratorStatus {
	t.Helper()

	ports, err := node.Sidecars[0].GetHostPorts(ctx, "7070/tcp")
	require.NoError(t, err)
	require.Len(t, ports, 1)

	url := "http://" + ports[0] + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var status OrchestratorStatus
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(body, &status))

	return status
}
