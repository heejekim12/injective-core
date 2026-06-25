package helpers

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/mempool"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/gogoproto/proto"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/dockerutil"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/stretchr/testify/require"
)

const (
	// DefaultGas is the default gas limit for standard transactions
	DefaultGas = 300_000
	// ProposalGas is the gas limit for governance proposal transactions
	ProposalGas = 400_000

	// ValidatorKeyName is the key name used for validator accounts in the node's keyring
	ValidatorKeyName = "validator"
)

// Tx contains some of Cosmos transaction details.
type Tx struct {
	Height uint64
	TxHash string

	GasWanted uint64
	GasUsed   uint64

	ErrorCode uint32
}

type Sender struct {
	User        ibc.Wallet
	Broadcaster *cosmos.Broadcaster
	TxFactory   *tx.Factory
	accSeq      int
}

func NewSender(t *testing.T, ctx context.Context, user ibc.Wallet, chain *cosmos.CosmosChain) *Sender {
	broadcaster := cosmos.NewBroadcaster(t, chain)
	txFactory, _ := broadcaster.GetFactory(ctx, user)
	txFactory = txFactory.WithSimulateAndExecute(false)
	return &Sender{
		User:        user,
		TxFactory:   &txFactory,
		Broadcaster: broadcaster,
	}
}

func (s *Sender) SendTx(ctx context.Context, wrapAuthz bool, gasLimit *uint64, msgs ...types.Msg) (string, error) {
	if wrapAuthz {
		encodedMsgs := make([]*codectypes.Any, 0, len(msgs))
		for _, msg := range msgs {
			encMsg, err := codectypes.NewAnyWithValue(msg)
			if err != nil {
				return "", err
			}
			encodedMsgs = append(encodedMsgs, encMsg)
		}

		msgs = []types.Msg{&authz.MsgExec{
			Grantee: s.User.FormattedAddress(),
			Msgs:    encodedMsgs,
		}}
	}

	gl := uint64(2_000_000)
	if gasLimit != nil {
		gl = *gasLimit
	}
	txFactory := s.TxFactory.WithGas(gl)
	s.TxFactory = &txFactory

	for {
		txResponse, err := BroadcastTxAsync(
			ctx,
			s.Broadcaster,
			s.TxFactory.WithSequence(uint64(s.accSeq)),
			s.User,
			msgs...,
		)
		if err != nil {
			if !strings.Contains(err.Error(), mempool.ErrMempoolTxMaxCapacity.Error()) {
				return "", err
			}
			// if mempool is full, wait 500ms for it to be flushed in the next
			// committed block
			time.Sleep(500 * time.Millisecond)
			continue
		}
		s.accSeq++
		return txResponse.TxHash, nil
	}
}

// BroadcastTxAsync broadcasts a transaction and returns immediately, without
// waiting for the transaction to be committed to state.
func BroadcastTxAsync(
	ctx context.Context,
	broadcaster *cosmos.Broadcaster,
	txFactory tx.Factory,
	user cosmos.User,
	msgs ...types.Msg,
) (*types.TxResponse, error) {
	clientCtx, err := broadcaster.GetClientContext(ctx, user)
	if err != nil {
		return nil, err
	}

	err = tx.BroadcastTx(clientCtx, txFactory, msgs...)
	if err != nil {
		return nil, err
	}

	txBytes, err := broadcaster.GetTxResponseBytes(ctx, user)
	if err != nil {
		return nil, err
	}

	txResponse, err := broadcaster.UnmarshalTxResponseBytes(ctx, txBytes)

	return &txResponse, err
}

// MustBroadcastMsg broadcasts a transaction and ensures it is valid, failing the test if it is not.
func MustBroadcastMsg(t *testing.T, chain *cosmos.CosmosChain, ctx context.Context, broadcastingUser ibc.Wallet, msg types.Msg, gas uint64) {
	broadcaster := cosmos.NewBroadcaster(t, chain)
	broadcaster.ConfigureFactoryOptions(WithGas(gas))

	txFactory, err := broadcaster.GetFactory(ctx, broadcastingUser)
	require.NoError(t, err, "error getting transaction factory")

	txResponse, err := BroadcastTxAsync(
		ctx,
		broadcaster,
		txFactory,
		broadcastingUser,
		msg,
	)
	require.NoError(t, err, "error broadcasting txs")
	require.EqualValues(t, 0, txResponse.Code)
}

// MustFailMsg broadcasts a transaction and ensures it fails with the expected error message, failing the test if it succeeds.
func MustFailMsg(t *testing.T, chain *cosmos.CosmosChain, ctx context.Context, broadcastingUser ibc.Wallet, msg types.Msg, errorMsg string, gas uint64) {
	broadcaster := cosmos.NewBroadcaster(t, chain)
	broadcaster.ConfigureFactoryOptions(WithGas(gas))

	txFactory, err := broadcaster.GetFactory(ctx, broadcastingUser)
	require.NoError(t, err, "error getting transaction factory")

	txResponse, err := BroadcastTxAsync(
		ctx,
		broadcaster,
		txFactory,
		broadcastingUser,
		msg,
	)
	require.NoError(t, err, errorMsg)

	fullTransaction, err := QueryTxRPC(ctx, chain.Nodes()[0], txResponse.TxHash)
	require.Error(t, err)
	require.NotEqual(t, 0, fullTransaction.ErrorCode)
	require.Contains(t, err.Error(), errorMsg)
}

func MustSucceedProposal(t *testing.T, chain *cosmos.CosmosChain, ctx context.Context, user ibc.Wallet, proposal proto.Message, proposalName string) {
	proposalEncoded, err := codectypes.NewAnyWithValue(
		proposal,
	)
	require.NoError(t, err, "failed to pack proposal", proposalName)

	broadcaster := cosmos.NewBroadcaster(t, chain)
	proposalInitialDeposit := math.NewIntWithDecimal(1000, 18)

	broadcaster.ConfigureFactoryOptions(WithGas(ProposalGas))
	txFactory, err := broadcaster.GetFactory(ctx, user)
	require.NoError(t, err, "error getting transaction factory")

	txResp, err := BroadcastTxAsync(
		ctx,
		broadcaster,
		txFactory,
		user,
		&govv1.MsgSubmitProposal{
			InitialDeposit: []types.Coin{types.NewCoin(
				chain.Config().Denom,
				proposalInitialDeposit,
			)},
			Proposer: user.FormattedAddress(),
			Title:    proposalName,
			Summary:  proposalName,
			Messages: []*codectypes.Any{
				proposalEncoded,
			},
		},
	)
	require.NoError(t, err, "error submitting proposal tx", proposalName)

	proposalTx, err := QueryProposalTx(context.Background(), chain.Nodes()[0], txResp.TxHash)
	require.NoError(t, err, "error checking proposal tx", proposalName)
	proposalID, err := strconv.ParseUint(proposalTx.ProposalID, 10, 64)
	require.NoError(t, err, "error parsing proposal ID", proposalName)

	_, err = VoteOnProposalAllValidatorsRPC(t, ctx, chain, proposalID, govv1.VoteOption_VOTE_OPTION_YES)
	require.NoError(t, err, "failed to submit proposal votes", proposalName)

	_, err = WaitForProposalStatusByTime(ctx, chain, proposalID, govv1beta1.StatusPassed, 45*time.Second, 500*time.Millisecond)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks", proposalName)
}

func MustSucceedProposalFromContent(
	t *testing.T,
	chain *cosmos.CosmosChain,
	ctx context.Context,
	user ibc.Wallet,
	proposalContent govv1beta1.Content,
	proposalName string,
) {
	broadcaster := cosmos.NewBroadcaster(t, chain)
	broadcaster.ConfigureFactoryOptions(WithGas(ProposalGas))
	txFactory, err := broadcaster.GetFactory(ctx, user)
	require.NoError(t, err, "error getting transaction factory")

	p := &govv1beta1.MsgSubmitProposal{
		InitialDeposit: []types.Coin{types.NewCoin(
			chain.Config().Denom,
			math.NewIntWithDecimal(1000, 18),
		)},
		Proposer: user.FormattedAddress(),
	}
	require.NoError(t, p.SetContent(proposalContent))

	txResp, err := BroadcastTxAsync(
		ctx,
		broadcaster,
		txFactory,
		user,
		p,
	)
	require.NoError(t, err, "error submitting proposal tx", proposalName)

	minNotionalTx, err := QueryProposalTx(context.Background(), chain.Nodes()[0], txResp.TxHash)
	require.NoError(t, err, "error checking proposal tx", proposalName)
	proposalID, err := strconv.ParseUint(minNotionalTx.ProposalID, 10, 64)
	require.NoError(t, err, "error parsing proposal ID", proposalName)

	_, err = VoteOnProposalAllValidatorsRPC(t, ctx, chain, proposalID, govv1.VoteOption_VOTE_OPTION_YES)
	require.NoError(t, err, "failed to submit proposal votes", proposalName)

	_, err = WaitForProposalStatusByTime(ctx, chain, proposalID, govv1beta1.StatusPassed, 45*time.Second, 500*time.Millisecond)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks", proposalName)
}

// WithGas returns a cosmos.FactoryOpt that sets the gas limit for a transaction and disables simulation.
func WithGas(gas uint64) cosmos.FactoryOpt {
	return func(factory tx.Factory) tx.Factory {
		return factory.WithGas(gas).WithSimulateAndExecute(false)
	}
}

// WithFees returns a cosmos.FactoryOpt that sets the fees for a transaction and disables simulation.
// When providing explicit fees, gas prices must be set to empty to avoid conflict.
func WithFees(fees string) cosmos.FactoryOpt {
	return func(factory tx.Factory) tx.Factory {
		return factory.WithFees(fees).WithGasPrices("").WithSimulateAndExecute(false)
	}
}

// BroadcastTxBlock broadcasts a transaction with custom factory options and waits for it to be included in a block.
// Returns the ResultTx after the transaction is committed.
// Uses RPC queries for better performance compared to CLI-based queries.
func BroadcastTxBlock(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user ibc.Wallet,
	factoryOpts []cosmos.FactoryOpt,
	msgs ...types.Msg,
) *types.TxResponse {
	t.Helper()

	require.NotNil(t, factoryOpts, "BroadcastTxBlock requires factoryOpts")

	broadcaster := cosmos.NewBroadcaster(t, chain)
	broadcaster.ConfigureFactoryOptions(factoryOpts...)

	// Get the transaction factory with the configured options
	txFactory, err := broadcaster.GetFactory(ctx, user)
	require.NoError(t, err, "error getting transaction factory")

	// Broadcast transaction asynchronously - returns immediately after mempool inclusion
	resp, err := BroadcastTxAsync(ctx, broadcaster, txFactory, user, msgs...)
	require.NoError(t, err, "error broadcasting tx asynchronously")

	// Query the transaction via gRPC to get the full response
	resultTx, err := getTxResponseRPC(ctx, chain.Nodes()[0], resp.TxHash)
	require.NoError(t, err, "error querying tx after broadcast")

	return resultTx
}

// VoteOnProposalAllValidatorsRPC votes on a proposal with all validators using gRPC.
// This function broadcasts vote transactions entirely via gRPC instead of using CLI commands.
// It exports the validator private key from each node's keyring and uses it to sign and broadcast.
func VoteOnProposalAllValidatorsRPC(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	proposalID uint64,
	voteOption govv1.VoteOption,
) ([]string, error) {
	t.Helper()

	// Get all validator nodes
	nodes := chain.Validators
	txHashes := make([]string, 0, len(nodes))

	// Vote with each validator (async - we'll wait for the proposal status separately)
	for _, node := range nodes {
		txHash, err := BroadcastMsgWithKeyringAsync(ctx, chain, node, ValidatorKeyName, DefaultGas, func(voter types.AccAddress) ([]types.Msg, error) {
			return []types.Msg{
				govv1.NewMsgVote(
					voter,
					proposalID,
					voteOption,
					"", // metadata
				),
			}, nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to broadcast vote from validator on node %s: %w", node.Name(), err)
		}
		txHashes = append(txHashes, txHash)
	}

	return txHashes, nil
}

// BroadcastMsgWithKeyringAsync broadcasts a message using the node's keyring for signing via gRPC.
// It returns immediately after the transaction is accepted into the mempool, without waiting
// for it to be committed to a block.
//
// This is a generic version that accepts any key name from the node's keyring.
func BroadcastMsgWithKeyringAsync(
	ctx context.Context,
	chain *cosmos.CosmosChain,
	node *cosmos.ChainNode,
	keyName string,
	gas uint64,
	msgBuilder func(sender types.AccAddress) ([]types.Msg, error),
) (string, error) {
	tempDir, err := os.MkdirTemp("", "ict-keyring-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary keyring dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	containerKeyringDir := path.Join(node.HomeDir(), "keyring-test")
	cosmosKeyring, err := dockerutil.NewLocalKeyringFromDockerContainer(
		ctx,
		chain.GetCodec(),
		node.DockerClient,
		tempDir,
		containerKeyringDir,
		node.ContainerID(),
		chain.Config().KeyringOptions...,
	)
	if err != nil {
		return "", fmt.Errorf("failed to copy keyring from node %s: %w", node.Name(), err)
	}

	keyInfo, err := cosmosKeyring.Key(keyName)
	if err != nil {
		return "", fmt.Errorf("failed to load key %s from keyring: %w", keyName, err)
	}
	senderAddress, err := keyInfo.GetAddress()
	if err != nil {
		return "", fmt.Errorf("failed to resolve address from keyring: %w", err)
	}

	msgs, err := msgBuilder(senderAddress)
	if err != nil {
		return "", fmt.Errorf("failed to build messages for node %s: %w", node.Name(), err)
	}

	// Create client context using sdk-go helper (handles codec registration internally)
	clientCtx, err := chainclient.NewClientContext(
		chain.Config().ChainID,
		senderAddress.String(),
		cosmosKeyring,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create client context: %w", err)
	}

	// Setup CometBFT RPC client
	tmClient, err := rpchttp.New(chain.GetHostRPCAddress())
	if err != nil {
		return "", fmt.Errorf("failed to create CometBFT RPC client: %w", err)
	}
	defer func() {
		if closer, ok := interface{}(tmClient).(interface{ Close() }); ok {
			closer.Close() // Ensure HTTP connections are cleaned up if Close method exists
		}
	}()
	clientCtx = clientCtx.WithNodeURI(chain.GetHostRPCAddress()).WithClient(tmClient).WithSimulation(false)

	// Create tx factory with the specified gas (no simulation)
	txFactory := chainclient.NewTxFactory(clientCtx)

	gasPrices := chain.Config().GasPrices
	if gasPrices == "" {
		gasPrices = fmt.Sprintf("%d%s", 160_000_000, InjectiveBondDenom)
	}

	txFactory = txFactory.WithGas(gas).WithGasPrices(gasPrices)

	// Create chain client
	network := common.Network{
		ChainId:                 chain.Config().ChainID,
		TmEndpoint:              chain.GetHostRPCAddress(),
		ChainGrpcEndpoint:       chain.GetHostGRPCAddress(),
		ChainCookieAssistant:    &common.DisabledCookieAssistant{},
		ExchangeCookieAssistant: &common.DisabledCookieAssistant{},
		ExplorerCookieAssistant: &common.DisabledCookieAssistant{},
	}

	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		network,
		common.OptionTxFactory(&txFactory),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create chain client: %w", err)
	}

	// Broadcast the message
	_, response, err := chainClient.BroadcastMsg(ctx, txtypes.BroadcastMode_BROADCAST_MODE_SYNC, msgs...)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast tx: %w", err)
	}

	if response.TxResponse.Code != 0 {
		return "", fmt.Errorf("tx failed with code %d: %s", response.TxResponse.Code, response.TxResponse.RawLog)
	}

	return response.TxResponse.TxHash, nil
}

func BroadcastRawTxBytesSync(
	ctx context.Context,
	chain *cosmos.CosmosChain,
	txBytes []byte,
) (string, error) {
	tmClient, err := rpchttp.New(chain.GetHostRPCAddress())
	if err != nil {
		return "", fmt.Errorf("failed to create CometBFT RPC client: %w", err)
	}
	defer func() {
		if closer, ok := interface{}(tmClient).(interface{ Close() }); ok {
			closer.Close()
		}
	}()

	res, err := tmClient.BroadcastTxSync(ctx, txBytes)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast raw tx: %w", err)
	}

	if res.Code != 0 {
		return "", fmt.Errorf(
			"tx failed with code %d: codespace=%s log=%s",
			res.Code,
			res.Codespace,
			res.Log,
		)
	}

	return fmt.Sprintf("%X", res.Hash), nil
}
