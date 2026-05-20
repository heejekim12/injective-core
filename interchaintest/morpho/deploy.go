package morpho

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/stretchr/testify/require"

	"github.com/InjectiveLabs/injective-core/interchaintest/foundry"
	"github.com/InjectiveLabs/injective-core/interchaintest/helpers"
)

const (
	WorkDir = "/apps/data"
	RepoURL = "https://github.com/InjectiveLabs/solidity-contracts.git"
	RepoRef = "v1.20.0"
	RepoDir = WorkDir + "/solidity-contracts"

	aggregatorArtifact = "out/InjectiveAggregatorV3.sol/InjectiveAggregatorV3.json"
	oracleArtifact     = "out/MorphoChainlinkOracleV2.sol/MorphoChainlinkOracleV2.json"
)

type FeedConfig struct {
	OracleType  uint8
	PriceID     string
	QuoteSymbol string
	Decimals    uint8
	Description string
}

type OracleWrapperConfig struct {
	BaseFeed           FeedConfig
	BaseTokenDecimals  uint8
	QuoteFeed          FeedConfig
	QuoteTokenDecimals uint8
}

type OracleWrapperSuite struct {
	BaseFeed     common.Address
	QuoteFeed    common.Address
	MorphoOracle common.Address
}

type foundryArtifact struct {
	ABI      json.RawMessage `json:"abi"`
	Bytecode struct {
		Object string `json:"object"`
	} `json:"bytecode"`
}

// SetupSolidityContractsRepo clones the solidity-contracts ref and runs forge build
// inside the deployer container.
func SetupSolidityContractsRepo(t *testing.T, ctx context.Context, deployer *foundry.Container) {
	t.Helper()

	t.Log("Setting up solidity-contracts repository in deployer container...")

	stdout, stderr, err := deployer.Exec(
		ctx,
		[]string{
			"bash", "-lc",
			fmt.Sprintf("cd %s && git clone --depth 1 --single-branch --branch %s %s solidity-contracts", WorkDir, RepoRef, RepoURL),
		},
	)
	if err != nil {
		if stdout != "" {
			t.Logf("git clone stdout:\n%s", stdout)
		}
		if stderr != "" {
			t.Logf("git clone stderr:\n%s", stderr)
		}
		require.NoError(t, err, "failed to clone solidity-contracts repository")
	}

	stdout, stderr, err = deployer.Exec(
		ctx,
		[]string{
			"bash", "-lc",
			fmt.Sprintf("cd %s && forge build", RepoDir),
		},
	)
	if err != nil {
		if stdout != "" {
			t.Logf("forge build stdout:\n%s", stdout)
		}
		if stderr != "" {
			t.Logf("forge build stderr:\n%s", stderr)
		}
		require.NoError(t, err, "failed to build solidity-contracts in foundry container")
	}
}

// DeployMorphoOracleWrapperSuite deploys the InjectiveAggregatorV3 wrappers and
// MorphoChainlinkOracleV2 against the running chain.
func DeployMorphoOracleWrapperSuite(
	t *testing.T,
	ctx context.Context,
	ethClient *ethclient.Client,
	chainID *big.Int,
	deployerWallet ibc.Wallet,
	config OracleWrapperConfig,
	deployer *foundry.Container,
) OracleWrapperSuite {
	t.Helper()

	baseFeed := deployContractFromArtifact(
		t,
		ctx,
		ethClient,
		chainID,
		deployerWallet,
		deployer,
		aggregatorArtifact,
		config.BaseFeed.OracleType,
		config.BaseFeed.PriceID,
		config.BaseFeed.QuoteSymbol,
		config.BaseFeed.Decimals,
		config.BaseFeed.Description,
	)

	t.Log("deployed base feed aggregator contract:", baseFeed.Hex())

	quoteFeed := deployContractFromArtifact(
		t,
		ctx,
		ethClient,
		chainID,
		deployerWallet,
		deployer,
		aggregatorArtifact,
		config.QuoteFeed.OracleType,
		config.QuoteFeed.PriceID,
		config.QuoteFeed.QuoteSymbol,
		config.QuoteFeed.Decimals,
		config.QuoteFeed.Description,
	)

	t.Log("deployed quote feed aggregator contract:", quoteFeed.Hex())

	morphoOracle := deployContractFromArtifact(
		t,
		ctx,
		ethClient,
		chainID,
		deployerWallet,
		deployer,
		oracleArtifact,
		common.Address{},
		big.NewInt(1),
		baseFeed,
		common.Address{},
		big.NewInt(int64(config.BaseTokenDecimals)),
		common.Address{},
		big.NewInt(1),
		quoteFeed,
		common.Address{},
		big.NewInt(int64(config.QuoteTokenDecimals)),
	)

	t.Log("deployed morpho oracle contract:", morphoOracle.Hex())

	return OracleWrapperSuite{
		BaseFeed:     baseFeed,
		QuoteFeed:    quoteFeed,
		MorphoOracle: morphoOracle,
	}
}

func deployContractFromArtifact(
	t *testing.T,
	ctx context.Context,
	ethClient *ethclient.Client,
	chainID *big.Int,
	deployerWallet ibc.Wallet,
	deployer *foundry.Container,
	artifactName string,
	constructorArgs ...interface{},
) common.Address {
	t.Helper()

	meta := loadArtifactMetadata(t, ctx, deployer, artifactName)
	parsed, err := meta.GetAbi()
	require.NoError(t, err)
	require.NotNil(t, parsed)

	privateKey, err := helpers.GenerateEthereumPrivateKey(deployerWallet.Mnemonic())
	require.NoError(t, err)

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	require.NoError(t, err)
	auth.Context = ctx
	auth.GasPrice = big.NewInt(1)

	address, tx, _, err := bind.DeployContract(auth, *parsed, common.FromHex(meta.Bin), ethClient, constructorArgs...)
	require.NoError(t, err)

	receipt, err := bind.WaitMined(ctx, ethClient, tx)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.ReceiptStatusSuccessful, receipt.Status)
	require.Equal(t, address, receipt.ContractAddress)

	return address
}

func loadArtifactMetadata(
	t *testing.T,
	ctx context.Context,
	deployer *foundry.Container,
	artifactRelPath string,
) *bind.MetaData {
	t.Helper()

	artifactPath := filepath.ToSlash(filepath.Join(RepoDir, filepath.FromSlash(artifactRelPath)))
	stdout, stderr, err := deployer.Exec(ctx, []string{"cat", artifactPath})
	if err != nil {
		if stdout != "" {
			t.Logf("artifact stdout:\n%s", stdout)
		}
		if stderr != "" {
			t.Logf("artifact stderr:\n%s", stderr)
		}
		require.NoErrorf(t, err, "failed to read artifact %s from foundry container", artifactRelPath)
	}

	var artifact foundryArtifact
	require.NoError(t, json.Unmarshal([]byte(stdout), &artifact))
	require.NotEmpty(t, artifact.ABI)
	require.NotEmpty(t, artifact.Bytecode.Object)

	return &bind.MetaData{
		ABI: string(artifact.ABI),
		Bin: artifact.Bytecode.Object,
	}
}
