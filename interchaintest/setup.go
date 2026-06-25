package interchaintest

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	chaincodec "github.com/InjectiveLabs/sdk-go/chain/codec"
	erc20types "github.com/InjectiveLabs/sdk-go/chain/erc20/types"
	evmtypes "github.com/InjectiveLabs/sdk-go/chain/evm/types"
	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types"
	exchangev2types "github.com/InjectiveLabs/sdk-go/chain/exchange/types/v2"
	insurancetypes "github.com/InjectiveLabs/sdk-go/chain/insurance/types"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	peggytypes "github.com/InjectiveLabs/sdk-go/chain/peggy/types"
	permissionstypes "github.com/InjectiveLabs/sdk-go/chain/permissions/types"
	tokenfactorytypes "github.com/InjectiveLabs/sdk-go/chain/tokenfactory/types"
	wasmxtypes "github.com/InjectiveLabs/sdk-go/chain/wasmx/types"
	"github.com/avast/retry-go/v4"
	probabilistic "github.com/cardano-foundation/cardano-ibc-incubator/cosmos/cardano-probabilistic-light-client-v8"
	cosmtestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/ethereum/go-ethereum/crypto"
	dockerclient "github.com/moby/moby/client"
	"github.com/strangelove-ventures/interchaintest/v8"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/chain/ethereum"
	"github.com/strangelove-ventures/interchaintest/v8/chain/ethereum/geth"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testreporter"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/InjectiveLabs/injective-core/interchaintest/helpers"
)

const (
	InjectiveGovVotingPeriod     = 30 * time.Second
	InjectiveGovMaxDepositPeriod = 20 * time.Second
	dockerBuildLockFileName      = "injective-interchaintest-docker.lock"
)

var (
	// InjectiveE2ERepo is the Docker image that is published as an official release
	InjectiveE2ERepo = "injectivelabs/injective-core"

	IBCRelayerImage   = "ghcr.io/cosmos/relayer"
	IBCRelayerVersion = "main"

	// InjectiveCoreImage is the Docker image that is staged for test (built from current branch)
	InjectiveCoreImage = ibc.DockerImage{
		Repository: "injectivelabs/injective-core",
		Version:    "local",
		UIDGID:     "1025:1025",
	}

	createChainBuildMu     sync.Mutex
	activeCreateChainTests sync.Map

	defaultGenesisOverridesKV = []cosmos.GenesisKV{
		{
			Key:   "app_state.gov.params.voting_period",
			Value: InjectiveGovVotingPeriod.String(),
		},
		{
			Key:   "app_state.gov.params.max_deposit_period",
			Value: InjectiveGovMaxDepositPeriod.String(),
		},
		{
			Key:   "app_state.gov.params.min_deposit.0.denom",
			Value: helpers.InjectiveBondDenom,
		},
	}
)

// injectiveEncoding registers the injectiveCore specific module codecs so that the associated types and msgs
// will be supported when writing to the blocksdb sqlite database.
func injectiveEncoding() *cosmtestutil.TestEncodingConfig {
	cfg := cosmos.DefaultEncoding()

	chaincodec.RegisterInterfaces(cfg.InterfaceRegistry)
	exchangetypes.RegisterInterfaces(cfg.InterfaceRegistry)
	exchangev2types.RegisterInterfaces(cfg.InterfaceRegistry)
	permissionstypes.RegisterInterfaces(cfg.InterfaceRegistry)
	wasmtypes.RegisterInterfaces(cfg.InterfaceRegistry)
	insurancetypes.RegisterInterfaces(cfg.InterfaceRegistry)
	oracletypes.RegisterInterfaces(cfg.InterfaceRegistry)
	peggytypes.RegisterInterfaces(cfg.InterfaceRegistry)
	tokenfactorytypes.RegisterInterfaces(cfg.InterfaceRegistry)
	wasmxtypes.RegisterInterfaces(cfg.InterfaceRegistry)
	authztypes.RegisterInterfaces(cfg.InterfaceRegistry)

	// TODO: types dependency shall be moved to sdk-go
	evmtypes.RegisterInterfaces(cfg.InterfaceRegistry)
	erc20types.RegisterInterfaces(cfg.InterfaceRegistry)
	probabilistic.RegisterInterfaces(cfg.InterfaceRegistry)

	return &cfg
}

// injectiveChainConfig returns dynamic config for injective chains, allowing to inject genesis overrides
func InjectiveChainConfig(
	genesisOverrides ...cosmos.GenesisKV,
) ibc.ChainConfig {
	// Merge genesisOverrides with defaults, where genesisOverrides take precedence
	// Start with defaults, then append overrides so they take precedence (last wins)
	mergedOverrides := make([]cosmos.GenesisKV, 0, len(defaultGenesisOverridesKV)+len(genesisOverrides))
	mergedOverrides = append(mergedOverrides, defaultGenesisOverridesKV...)
	mergedOverrides = append(mergedOverrides, genesisOverrides...)
	genesisOverrides = mergedOverrides

	consensusOverrides := make(testutil.Toml)
	consensusOverrides["timeout_propose"] = "1s"
	consensusOverrides["timeout_propose_delta"] = "100ms"
	consensusOverrides["timeout_prevote"] = "250ms"
	consensusOverrides["timeout_prevote_delta"] = "100ms"
	consensusOverrides["timeout_precommit"] = "250ms"
	consensusOverrides["timeout_precommit_delta"] = "100ms"
	consensusOverrides["timeout_commit"] = "500ms"
	consensusOverrides["double_sign_check_height"] = 0
	consensusOverrides["skip_timeout_commit"] = false
	consensusOverrides["create_empty_blocks"] = true
	consensusOverrides["create_empty_blocks_interval"] = "0s"
	consensusOverrides["peer_gossip_sleep_duration"] = "10ms"
	consensusOverrides["peer_query_maj23_sleep_duration"] = "2s"
	cometbftTomlOverrides := make(testutil.Toml)
	cometbftTomlOverrides["consensus"] = consensusOverrides

	jsonRpcOverrides := make(testutil.Toml)
	jsonRpcOverrides["address"] = "0.0.0.0:8545"
	jsonRpcOverrides["api"] = "eth,net,web3,debug,inj"

	// Configure websocket server for chainstream
	websocketOverrides := make(testutil.Toml)
	websocketOverrides["address"] = "0.0.0.0:9998"
	websocketOverrides["max-open-connections"] = 100

	chainStreamOverrides := make(testutil.Toml)
	chainStreamOverrides["server"] = "0.0.0.0:9999"

	appTomlOverrides := make(testutil.Toml)
	appTomlOverrides["json-rpc"] = jsonRpcOverrides
	appTomlOverrides["chainstream"] = chainStreamOverrides
	appTomlOverrides["injective-websocket"] = websocketOverrides

	config := ibc.ChainConfig{
		Type: "cosmos",
		Name: "injective",
		Images: []ibc.DockerImage{
			InjectiveCoreImage,
		},
		ChainID:             "injtest-1",
		Bech32Prefix:        helpers.Bech32MainPrefix,
		Denom:               helpers.InjectiveBondDenom,
		SigningAlgorithm:    helpers.InjectiveSigningAlgorithm,
		CoinDecimals:        &helpers.InjectiveCoinDecimals,
		CoinType:            fmt.Sprintf("%d", helpers.InjectiveCoinType),
		GasPrices:           fmt.Sprintf("1%s", helpers.InjectiveBondDenom),
		GasAdjustment:       1.5,
		TrustingPeriod:      "112h",
		UsingChainIDFlagCLI: true,
		EncodingConfig:      injectiveEncoding(),
		CryptoCodec:         helpers.InjectiveCryptoCodec(),
		KeyringOptions:      helpers.InjectiveKeyringOptions,
		ModifyGenesis:       cosmos.ModifyGenesis(genesisOverrides),
		ExposeAdditionalPorts: []string{
			"8545/tcp", // open the port for the EVM on all nodes
			"9999/tcp", // open the port for the chainstream server on all nodes
			"9998/tcp", // open the port for the websocket server on all nodes
		},
		ConfigFileOverrides: map[string]any{
			"config/app.toml":    appTomlOverrides,
			"config/config.toml": cometbftTomlOverrides,
		},
		Bin:         "injectived",
		NoHostMount: false,
	}

	if os.Getenv("DO_COVERAGE") == "true" || os.Getenv("DO_COVERAGE") == "yes" {
		config.Env = append(config.Env, "GOCOVERDIR=/apps/data/coverage")
	}

	return config
}

func GethChainConfig() ibc.ChainConfig {
	// Warning: do not use default config, have the params adjustable there.
	return ibc.ChainConfig{
		Type:           "ethereum",
		Name:           "ethereum",
		ChainID:        "1337", // default geth chain-id
		Bech32Prefix:   "n/a",
		CoinType:       "60",
		Denom:          "wei",
		GasPrices:      "2000000000", // 2gwei, default 1M
		GasAdjustment:  0,
		TrustingPeriod: "0",
		NoHostMount:    false,
		Images: []ibc.DockerImage{
			{
				Repository: "ethereum/client-go",
				Version:    "v1.14.7",
				UIDGID:     "1025:1025",
			},
		},
		Bin: "geth",
		AdditionalStartArgs: []string{
			"--dev.period", "2", // 2 second block time
			"--verbosity", "4", // Level = debug
			"--networkid", "15",
			"--rpc.txfeecap", "50.0", // 50 eth
			"--rpc.gascap", "30000000", // 30M
			"--gpo.percentile", "150", // default 60
			"--gpo.ignoreprice", "1000000000", // 1gwei, default 2
			"--dev.gaslimit", "30000000", // 30M, default 11.5M
			"--rpc.enabledeprecatedpersonal", // required (in this version) for recover key and unlocking accounts
		},
	}
}

// SetupDocker serializes the Docker port-allocation window across both
// parallel tests in the current process and concurrent test processes on the
// same host. The returned release function is idempotent and is also
// registered with t.Cleanup.
func SetupDocker(t *testing.T) (*dockerclient.Client, string, func()) {
	t.Helper()

	createChainBuildMu.Lock()

	lockPath := filepath.Join(os.TempDir(), dockerBuildLockFileName)
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		createChainBuildMu.Unlock()
	}
	require.NoError(t, err)

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		_ = lockFile.Close()
		createChainBuildMu.Unlock()
	}
	require.NoError(t, err)

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
			_ = lockFile.Close()
			createChainBuildMu.Unlock()
		})
	}
	t.Cleanup(release)

	client, network := interchaintest.DockerSetup(t)
	return client, network, release
}

func WithDockerSetupLock(t *testing.T, fn func(client *dockerclient.Client, network string)) {
	t.Helper()

	client, network, _ := SetupDocker(t)
	fn(client, network)
}

// CreateChain boots an Injective interchaintest network with fixed extra host
// ports. It must be called at most once per test name because the global port
// binding lock is held until test cleanup runs, after the caller's ic.Close
// cleanup has had a chance to release Docker resources.
func CreateChain(
	t *testing.T,
	ctx context.Context,
	numVals, numFull int,
	chainPreStartNodes func(*cosmos.CosmosChain),
	genesisOverrides ...cosmos.GenesisKV,
) (*interchaintest.Interchain, *cosmos.CosmosChain) {
	t.Helper()

	if _, loaded := activeCreateChainTests.LoadOrStore(t.Name(), struct{}{}); loaded {
		t.Fatalf("CreateChain can only be called once per test name; use a subtest for an additional chain bootstrap")
		return nil, nil
	}
	t.Cleanup(func() {
		activeCreateChainTests.Delete(t.Name())
	})

	falseBool := false
	var (
		ic    *interchaintest.Interchain
		chain *cosmos.CosmosChain
	)

	buildChainAttempt := func() error {
		cf := interchaintest.NewBuiltinChainFactory(
			zaptest.NewLogger(t),
			[]*interchaintest.ChainSpec{
				{
					Name:          "injective",
					ChainName:     "injective",
					Version:       InjectiveCoreImage.Version,
					ChainConfig:   InjectiveChainConfig(genesisOverrides...),
					NumValidators: &numVals,
					NumFullNodes:  &numFull,
					NoHostMount:   &falseBool,
				},
			},
		)

		chains, err := cf.Chains(t.Name())
		if err != nil {
			return retry.Unrecoverable(err)
		}

		attemptChain := chains[0].(*cosmos.CosmosChain)
		if chainPreStartNodes != nil {
			attemptChain.WithPreStartNodes(chainPreStartNodes)
		}

		attemptIC := interchaintest.NewInterchain().AddChain(attemptChain)

		client, network, releaseDockerSetup := SetupDocker(t)
		err = attemptIC.Build(
			ctx,
			testreporter.NewNopReporter().RelayerExecReporter(t),
			interchaintest.InterchainBuildOptions{
				TestName:         t.Name(),
				Client:           client,
				NetworkID:        network,
				SkipPathCreation: true,
			},
		)
		if err != nil {
			_ = attemptIC.Close()
			releaseDockerSetup()
			if !isRetryableDockerPortBindError(err) {
				return retry.Unrecoverable(err)
			}

			return err
		}

		ic = attemptIC
		chain = attemptChain
		return nil
	}

	retryOpts := []retry.Option{
		retry.Attempts(3),
		retry.Delay(500 * time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxDelay(2 * time.Second),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			t.Logf("retrying injective chain bootstrap after docker port bind error (attempt %d): %v", n+2, err)
		}),
	}

	err := retry.Do(buildChainAttempt, retryOpts...)
	require.NoError(t, err)

	return ic, chain
}

func isRetryableDockerPortBindError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "port is already allocated") &&
		strings.Contains(msg, "failed to set up container networking")
}

const (
	peggoRateLimitAdminMnemonic = "cube teach job unknown manage evidence whisper moon zebra name derive kite venue fit galaxy nephew expose badge pride cake crash adjust derive provide"
	peggoRateLimitAdminAddress  = "inj1ps9tafd33sdu5fc4xgkj54cxwp6u7cx74vur0l"
)

func WireUpPeggo(
	t *testing.T,
	ctx context.Context,
	cosmosChain *cosmos.CosmosChain,
	gethChain *geth.GethChain,
) helpers.PeggyContractSuite {
	t.Helper()

	v1, err := cosmosChain.Nodes()[0].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)
	v2, err := cosmosChain.Nodes()[1].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)
	v3, err := cosmosChain.Nodes()[2].AccountKeyBech32(ctx, "validator")
	require.NoError(t, err)

	ethPk1, err := crypto.GenerateKey()
	require.NoError(t, err)
	ethPk2, err := crypto.GenerateKey()
	require.NoError(t, err)
	ethPk3, err := crypto.GenerateKey()
	require.NoError(t, err)

	eth1 := crypto.PubkeyToAddress(ethPk1.PublicKey).String()
	eth2 := crypto.PubkeyToAddress(ethPk2.PublicKey).String()
	eth3 := crypto.PubkeyToAddress(ethPk3.PublicKey).String()

	nodes := cosmosChain.Nodes()
	helpers.RegisterOrchestrator(t, ctx, cosmosChain, nodes[0], v1, eth1)
	helpers.RegisterOrchestrator(t, ctx, cosmosChain, nodes[1], v2, eth2)
	helpers.RegisterOrchestrator(t, ctx, cosmosChain, nodes[2], v3, eth3)
	time.Sleep(1 * time.Second)

	vs := helpers.GetCurrentValset(t, ctx, cosmosChain)

	ethWalletAmount := ibc.WalletAmount{
		Denom:  gethChain.Config().Denom,
		Amount: ethereum.ETHER.MulRaw(1000),
	}

	ethWalletAmount.Address = eth1
	require.NoError(t, gethChain.SendFunds(ctx, interchaintest.FaucetAccountKeyName, ethWalletAmount))

	ethWalletAmount.Address = eth2
	require.NoError(t, gethChain.SendFunds(ctx, interchaintest.FaucetAccountKeyName, ethWalletAmount))

	ethWalletAmount.Address = eth3
	require.NoError(t, gethChain.SendFunds(ctx, interchaintest.FaucetAccountKeyName, ethWalletAmount))

	time.Sleep(1 * time.Second)
	t.Log("minted funds for validators on ethereum")

	// 3. Deploy Peggy contract suite (using etherman)
	contracts := helpers.DeployPeggyContractSuite(t, ctx, gethChain, vs)

	gethID, err := strconv.Atoi(gethChain.Config().ChainID)
	require.NoError(t, err)

	// update peggy module params
	params := &peggytypes.Params{
		PeggyId:                       "injective-peggyid",
		ContractSourceHash:            "don't need this",
		BridgeEthereumAddress:         contracts.TransparentProxy.String(),
		BridgeChainId:                 uint64(gethID),
		SignedValsetsWindow:           100_000,
		SignedBatchesWindow:           100_000,
		SignedClaimsWindow:            100_000,
		TargetBatchTimeout:            43_200_000,
		AverageBlockTime:              1_000,
		AverageEthereumBlockTime:      2_000,
		SlashFractionValset:           math.LegacyMustNewDecFromStr("0.001000000000000000"),
		SlashFractionBatch:            math.LegacyMustNewDecFromStr("0.001000000000000000"),
		SlashFractionClaim:            math.LegacyMustNewDecFromStr("0.001000000000000000"),
		SlashFractionConflictingClaim: math.LegacyMustNewDecFromStr("0.001000000000000000"),
		UnbondSlashingValsetsWindow:   25_000,
		SlashFractionBadEthSignature:  math.LegacyMustNewDecFromStr("0.001000000000000000"),
		CosmosCoinDenom:               cosmosChain.Config().Denom,
		CosmosCoinErc20Contract:       contracts.InjectiveCoin.String(),
		ClaimSlashingEnabled:          false,
		BridgeContractStartHeight:     contracts.StartHeight,
		Admins:                        []string{authtypes.NewModuleAddress(govtypes.ModuleName).String(), peggoRateLimitAdminAddress},
		SegregatedWalletAddress:       "inj1dqryh824u0w7p6ajk2gsr29tgj6d0nkfwsgs46",
	}

	helpers.UpdatePeggyParams(t, ctx, cosmosChain, params)

	ethPKs := []*ecdsa.PrivateKey{ethPk1, ethPk2, ethPk3}

	for i, node := range nodes {
		cosmosPK := helpers.GetValidatorPrivateKey(t, ctx, node)
		ethPK := hex.EncodeToString(crypto.FromECDSA(ethPKs[i]))
		peggoDefaults := helpers.GetPeggoEnvDefaults(
			cosmosChain,
			gethChain,
			cosmosPK,
			ethPK,
			contracts.TransparentProxy,
		)

		err = node.NewSidecarProcess(ctx,
			false,
			"peggo",
			node.DockerClient,
			node.NetworkID,
			InjectiveCoreImage,
			node.HomeDir(),
			[]string{"7070/tcp"},
			[]string{"peggo", "orchestrator"},
			peggoDefaults,
		)
		require.NoError(t, err)
	}

	require.NoError(t, cosmosChain.StartAllValSidecars(ctx))
	time.Sleep(1 * time.Second)

	t.Log("peggo sidecars started")

	// Once Peggo is wired up, wait until the first relayed valset update is observed.
	latestValset := helpers.GetPeggyModuleState(t, ctx, cosmosChain).GetValsets()[0]
	helpers.AwaitLastObservedValsetNonce(t, ctx, 1*time.Minute, cosmosChain, latestValset.Nonce)

	return contracts
}
