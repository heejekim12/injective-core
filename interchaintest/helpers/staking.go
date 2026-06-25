package helpers

import (
	"context"
	"testing"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// QueryAllValidators lists all validators using gRPC.
func QueryAllValidators(t *testing.T, ctx context.Context, chain *cosmos.CosmosChain) []stakingtypes.Validator {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := stakingtypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.Validators, &stakingtypes.QueryValidatorsRequest{})
	require.NoError(t, err, "error querying validators")

	return resp.Validators
}

// QueryValidator gets info about particular validator using gRPC.
func QueryValidator(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	valoperAddr string,
) stakingtypes.Validator {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := stakingtypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.Validator, &stakingtypes.QueryValidatorRequest{
		ValidatorAddr: valoperAddr,
	})
	require.NoError(t, err, "error querying validator")

	return resp.Validator
}

// QueryDelegation gets info about particular delegation using gRPC.
func QueryDelegation(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	delegatorAddr string,
	valoperAddr string,
) stakingtypes.Delegation {
	t.Helper()

	delegation, ok := QueryDelegationOrNil(t, ctx, chain, delegatorAddr, valoperAddr)
	require.True(t, ok, "delegation not found for delegator %s validator %s", delegatorAddr, valoperAddr)

	return delegation
}

// QueryDelegationOrNil queries a delegation and returns it along with a found flag.
// It returns (zero, false) when no delegation exists for the (delegator, validator) pair,
// which is useful for asserting that a delegation was removed (e.g. after a dust cleanup).
//
//revive:disable:context-as-argument // matches the (t, ctx, chain) convention of the sibling helpers in this file
func QueryDelegationOrNil(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	delegatorAddr string,
	valoperAddr string,
) (stakingtypes.Delegation, bool) {
	t.Helper()

	conn, err := grpc.NewClient(chain.GetHostGRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err, "failed to create gRPC connection")
	defer conn.Close()

	queryClient := stakingtypes.NewQueryClient(conn)
	resp, err := QueryRPC(ctx, queryClient.Delegation, &stakingtypes.QueryDelegationRequest{
		DelegatorAddr: delegatorAddr,
		ValidatorAddr: valoperAddr,
	})
	if err != nil {
		// The staking query returns a "NotFound" gRPC error when the delegation is absent.
		return stakingtypes.Delegation{}, false
	}
	if resp.GetDelegationResponse() == nil {
		return stakingtypes.Delegation{}, false
	}

	return resp.DelegationResponse.Delegation, true
}
