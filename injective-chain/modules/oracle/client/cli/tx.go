//nolint:staticcheck // deprecated gov proposal flags
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govcli "github.com/cosmos/cosmos-sdk/x/gov/client/cli"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cliflags "github.com/InjectiveLabs/injective-core/cli/flags"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
)

const (
	flagName                     = "name"
	flagSymbols                  = "symbols"
	flagAskCount                 = "ask-count"
	flagMinCount                 = "min-count"
	flagIBCVersion               = "ibc-version"
	flagRequestedValidatorCount  = "requested-validator-count"
	flagSufficientValidatorCount = "sufficient-validator-count"
	flagMinSourceCount           = "min-source-count"
	flagIBCPortID                = "port-id"
	flagChannel                  = "channel"
	flagPrepareGas               = "prepare-gas"
	flagExecuteGas               = "execute-gas"
	flagFeeLimit                 = "fee-limit"
	flagPacketTimeoutTimestamp   = "packet-timeout-timestamp"
	flagLegacyOracleScriptIDs    = "legacy-oracle-script-ids"
)

// NewTxCmd returns a root CLI command handler for certain modules/oracle transaction commands.
func NewTxCmd() *cobra.Command {
	txCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Oracle transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	txCmd.AddCommand(
		NewRelayPriceFeedPriceTxCmd(),
		NewRelayCoinbaseMessagesTxCmd(),
		NewGrantPriceFeederPrivilegeProposalTxCmd(),
		NewRevokePriceFeederPrivilegeProposalTxCmd(),
		NewGrantProviderPrivilegeProposalTxCmd(),
		NewRevokeProviderPrivilegeProposalTxCmd(),
		NewRelayProviderPricesProposalTxCmd(),
		NewGrantStorkPublisherPrivilegeProposalTxCmd(),
		NewRevokeStorkPublisherPrivilegeProposalTxCmd(),
		NewRelaySedaFastPricesTxCmd(),
	)
	return txCmd
}

func NewGrantPriceFeederPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-price-feeder-privilege-proposal [base] [quote] [relayers] [flags]",
		Args:  cobra.ExactArgs(3),
		Short: "Submit a proposal to grant price feeder privilege.",
		Long: `Submit a proposal to grant price feeder privilege.

		Example:
		$ %s tx oracle grant-price-feeder-privilege-proposal base quote relayer1,relayer2 --title="grant price feeder privilege" --description="XX" --deposit="1000000000000000000inj" --from=genesis --keyring-backend=file --yes
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			relayers := strings.Split(args[2], ",")

			content, err := grantPriceFeederPrivilegeProposalArgsToContent(cmd, args[0], args[1], relayers)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()

			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewRevokePriceFeederPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-price-feeder-privilege-proposal [base] [quote] [relayers] [flags]",
		Args:  cobra.ExactArgs(3),
		Short: "Submit a proposal to revoke price feeder privilege.",
		Long: `Submit a proposal to revoke price feeder privilege.

		Example:
		$ %s tx oracle revoke-price-feeder-privilege-proposal base quote relayer1,relayer2 --title="revoke price feeder privilege" --description="XX" --deposit="1000000000000000000inj" --from=genesis --keyring-backend=file --yes
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			relayers := strings.Split(args[2], ",")

			content, err := revokePriceFeederPrivilegeProposalArgsToContent(cmd, args[0], args[1], relayers)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()

			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewRelayPriceFeedPriceTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay-price-feed-price [base] [quote] [price] [flags]",
		Args:  cobra.ExactArgs(3),
		Short: "Relay price feed price",
		Long: `Relay price feed price.

		Example:
		$ %s tx oracle relay-price-feed-price inj usdt 25.00 --from=genesis --keyring-backend=file --yes
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			price, err := math.LegacyNewDecFromStr(args[2])
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()
			msg := &types.MsgRelayPriceFeedPrice{
				Sender: from.String(),
				Base:   []string{args[0]}, // BTC
				Quote:  []string{args[1]}, // USDT
				Price:  []math.LegacyDec{price},
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewRelayCoinbaseMessagesTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay-coinbase-messages [x] [x] [x] [flags]",
		Args:  cobra.ExactArgs(0),
		Short: "Relay coinbase messages",
		Long: `Relay coinbase messages.

		Example:
		$ %s tx oracle relay-coinbase-messages [x] [x] [x] --from=genesis --keyring-backend=file --yes
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()
			msg := &types.MsgRelayCoinbaseMessages{
				Sender: from.String(),
				Messages: [][]byte{
					common.FromHex("0x000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000607fe06c00000000000000000000000000000000000000000000000000000000000000c00000000000000000000000000000000000000000000000000000000cdd578cf00000000000000000000000000000000000000000000000000000000000000006707269636573000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000034254430000000000000000000000000000000000000000000000000000000000"), //nolint:revive // ok
					common.FromHex("0x000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000607fee4000000000000000000000000000000000000000000000000000000000000000c000000000000000000000000000000000000000000000000000000000891e9d880000000000000000000000000000000000000000000000000000000000000006707269636573000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000034554480000000000000000000000000000000000000000000000000000000000"), //nolint:revive // ok
					common.FromHex("0x000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000607fef3000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000056facc00000000000000000000000000000000000000000000000000000000000000067072696365730000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000358545a0000000000000000000000000000000000000000000000000000000000"), //nolint:revive // ok
				},
				Signatures: [][]byte{
					common.FromHex("0x755d64ab12b52711b6ed6cea26b4005fe44884546bc6fbcb0ca31fd369e90a6f856cd792fb473603af598cb9946d3a5ceb627b26074b0294dcefd8d0d8f171d9000000000000000000000000000000000000000000000000000000000000001c"), //nolint:revive // ok
					common.FromHex("0x18a821b64b1a100cc1ff68c5b2ba2fa40de6f7abeb49981366b359af9d9f131e0db75d82358cf4e5850c38bff62d626034464740ba5e222c3aeeb05ea51c59f3000000000000000000000000000000000000000000000000000000000000001b"), //nolint:revive // ok
					common.FromHex("0x946c8037ce20231cdde2bb30cea45f4a2f60916d4e3a28d6e9ee82ff6a83d6fcb44073ed9561bb8b0f54e6256234e50770eded2042582c81a99e78581873759a000000000000000000000000000000000000000000000000000000000000001c"), //nolint:revive // ok
				},
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewGrantProviderPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-provider-privilege-proposal [providerName] [relayers] --title [title] --description [desc] [flags]",
		Args:  cobra.ExactArgs(2),
		Short: "Submit a proposal to Grand a Provider Privilege",
		Long: `Submit a proposal to Grand a Provider Privilege.
			Example:
			$ %s tx oracle grant-provider-privilege-proposal 1 --from mykey
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			provider := args[0]
			relayers := strings.Split(args[1], ",")

			title, err := cmd.Flags().GetString(govcli.FlagTitle)
			if err != nil {
				return errors.New("Proposal Title is required (add --title flag)")
			}

			description, err := cmd.Flags().GetString(govcli.FlagDescription)
			if err != nil {
				return errors.New("Proposal Description is required (add --description flag)")
			}

			content := &types.GrantProviderPrivilegeProposal{
				Title:       title,
				Description: description,
				Provider:    provider,
				Relayers:    relayers,
			}

			from := clientCtx.GetFromAddress()
			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewRevokeProviderPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-provider-privilege-proposal [providerName] [relayers] --title [title] --desc [desc] [flags]",
		Args:  cobra.ExactArgs(2),
		Short: "Submit a proposal to Grand a Provider Privilege",
		Long: `Submit a proposal to Grand a Provider Privilege.
			Example:
			$ %s tx oracle grant-provider-privilege-proposal 1 --from mykey
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			provider := args[0]
			relayers := strings.Split(args[1], ",")

			title, err := cmd.Flags().GetString(govcli.FlagTitle)
			if err != nil {
				return errors.New("Proposal Title is required (add --title flag)")
			}

			description, err := cmd.Flags().GetString(govcli.FlagDescription)
			if err != nil {
				return errors.New("Proposal Description is required (add --description flag)")
			}

			content := &types.RevokeProviderPrivilegeProposal{
				Title:       title,
				Description: description,
				Provider:    provider,
				Relayers:    relayers,
			}

			from := clientCtx.GetFromAddress()
			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func NewRelayProviderPricesProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay-provider-prices [providerName] [symbol:prices] [flags]",
		Args:  cobra.ExactArgs(2),
		Short: "Relay prices for given symbols",
		Long: `Relay prices for given symbols.
			Example:
			$ %s tx oracle relay-provider-prices provider1 barmad:1,barman:0 --from mykey
		`,
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()
			provider := args[0]
			symbolPrices := strings.Split(args[1], ",")

			symbols := make([]string, len(symbolPrices))
			prices := make([]math.LegacyDec, len(symbolPrices))
			for i, symbolPriceStr := range symbolPrices {
				symbolPrice := strings.Split(symbolPriceStr, ":")
				symbols[i] = symbolPrice[0]
				price, err := math.LegacyNewDecFromStr(symbolPrice[1])
				if err != nil {
					return errors.New(fmt.Sprintf("Price for symbol %v incorrect (%v)", symbols[i], symbolPrice[1]))
				}
				prices[i] = price
			}

			content := &types.MsgRelayProviderPrices{
				Sender:   from.String(),
				Provider: provider,
				Symbols:  symbols,
				Prices:   prices,
			}

			if err := content.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), content)
		},
	}
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func grantPriceFeederPrivilegeProposalArgsToContent(cmd *cobra.Command, base, quote string, relayers []string) (govtypes.Content, error) {
	title, err := cmd.Flags().GetString(govcli.FlagTitle)
	if err != nil {
		return nil, err
	}

	description, err := cmd.Flags().GetString(govcli.FlagDescription)
	if err != nil {
		return nil, err
	}

	content := &types.GrantPriceFeederPrivilegeProposal{
		Title:       title,
		Description: description,
		Base:        base,
		Quote:       quote,
		Relayers:    relayers,
	}
	if err := content.ValidateBasic(); err != nil {
		return nil, err
	}
	return content, nil
}

func revokePriceFeederPrivilegeProposalArgsToContent(cmd *cobra.Command, base, quote string, relayers []string) (govtypes.Content, error) {
	title, err := cmd.Flags().GetString(govcli.FlagTitle)
	if err != nil {
		return nil, err
	}

	description, err := cmd.Flags().GetString(govcli.FlagDescription)
	if err != nil {
		return nil, err
	}

	content := &types.RevokePriceFeederPrivilegeProposal{
		Title:       title,
		Description: description,
		Base:        base,
		Quote:       quote,
		Relayers:    relayers,
	}
	if err := content.ValidateBasic(); err != nil {
		return nil, err
	}
	return content, nil
}

func convertStringToUint64Array(arg string) ([]uint64, error) {
	strs := strings.Split(arg, ",")
	rates := []uint64{}
	for _, str := range strs {
		rate, err := strconv.Atoi(str)
		if err != nil {
			return rates, err
		}
		rates = append(rates, uint64(rate))
	}
	return rates, nil
}

func NewGrantStorkPublisherPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-stork-publishers-privilege-proposal [publishers] [flags]",
		Args:  cobra.ExactArgs(1),
		Short: "Submit a proposal to grant stork publishers privilege.",
		Long: strings.TrimSpace(`Submit a proposal to grant stork publishers privilege.

Passing in publisher separated by commas would be parsed automatically to get pairs publishers.
Ex) Stork,0xf024a9aa110798e5cd0d698fba6523113eaa7fb2,dxFeed,0x2501f03dcf18c2c711040c2f3eff9e728463e3fa -> [(Stork, 0xf024a9aa110798e5cd0d698fba6523113eaa7fb2), (dxFeed1, 0x2501f03dcf18c2c711040c2f3eff9e728463e3fa)]

		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			publisherAddrs := strings.Split(args[0], " ")

			content, err := grantStorkPublisherPrivilegeProposalArgsToContent(cmd, publisherAddrs)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()

			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func grantStorkPublisherPrivilegeProposalArgsToContent(cmd *cobra.Command, publisherAddrs []string) (govtypes.Content, error) {
	title, err := cmd.Flags().GetString(govcli.FlagTitle)
	if err != nil {
		return nil, err
	}

	description, err := cmd.Flags().GetString(govcli.FlagDescription)
	if err != nil {
		return nil, err
	}

	content := &types.GrantStorkPublisherPrivilegeProposal{
		Title:           title,
		Description:     description,
		StorkPublishers: publisherAddrs,
	}

	return content, nil
}

func NewRevokeStorkPublisherPrivilegeProposalTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-stork-publishers-privilege-proposal [publishers] [flags]",
		Args:  cobra.ExactArgs(1),
		Short: "Submit a proposal to revoke stork publishers privilege.",
		Long: strings.TrimSpace(`Submit a proposal to revoke stork publishers privilege.

Passing in publisher separated by commas would be parsed automatically to get pairs publishers.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			publisherAddrs := strings.Split(args[0], " ")

			content, err := revokeStorkPublisherPrivilegeProposalArgsToContent(cmd, publisherAddrs)
			if err != nil {
				return err
			}

			from := clientCtx.GetFromAddress()

			depositStr, err := cmd.Flags().GetString(govcli.FlagDeposit)
			if err != nil {
				return err
			}
			deposit, err := sdk.ParseCoinsNormalized(depositStr)
			if err != nil {
				return err
			}

			msg, err := govtypes.NewMsgSubmitProposal(content, deposit, from)
			if err != nil {
				return err
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String(govcli.FlagTitle, "", "title of proposal")
	cmd.Flags().String(govcli.FlagDescription, "", "description of proposal")
	cmd.Flags().String(govcli.FlagDeposit, "", "deposit of proposal")

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}

func revokeStorkPublisherPrivilegeProposalArgsToContent(cmd *cobra.Command, publishersInfo []string) (govtypes.Content, error) {
	title, err := cmd.Flags().GetString(govcli.FlagTitle)
	if err != nil {
		return nil, err
	}

	description, err := cmd.Flags().GetString(govcli.FlagDescription)
	if err != nil {
		return nil, err
	}

	content := &types.RevokeStorkPublisherPrivilegeProposal{
		Title:           title,
		Description:     description,
		StorkPublishers: publishersInfo,
	}

	return content, nil
}

// NewRelaySedaFastPricesTxCmd builds and broadcasts a MsgRelaySedaFastPrices.
// The positional argument is a path to a JSON file that contains either a
// single SEDA Fast result envelope object or a JSON array of such objects
// (matching the "result" field of a feed.result / feed.execute WebSocket
// message).
func NewRelaySedaFastPricesTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay-seda-fast-prices [path-to-json]",
		Short: "Relay SEDA Fast price update envelopes from a JSON file",
		Long: strings.TrimSpace(`Read one or more SEDA Fast result JSON envelopes from a file and
relay them on-chain as a MsgRelaySedaFastPrices.

The file must contain either a single JSON object (one envelope) or a JSON
array of objects (batch). Each object is the "result" field of a
feed.result or feed.execute WebSocket message, i.e.
  { "_tag": "ExecuteResponse", "data": { ... } }

Example:
  injectived tx oracle relay-seda-fast-prices ./seda_result.json --from=relayer
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			raw, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading seda fast json file: %w", err)
			}

			var updates [][]byte
			raw = []byte(strings.TrimSpace(string(raw)))
			if len(raw) > 0 && raw[0] == '[' {
				// JSON array of envelopes
				var arr []json.RawMessage
				if err := json.Unmarshal(raw, &arr); err != nil {
					return fmt.Errorf("parsing seda fast json array: %w", err)
				}
				for _, item := range arr {
					updates = append(updates, item)
				}
			} else {
				// Single envelope
				updates = [][]byte{raw}
			}

			if len(updates) == 0 {
				return fmt.Errorf("no SEDA Fast updates found in file %s", args[0])
			}
			if len(updates) > types.MaxSedaFastUpdatesPerMsg {
				return fmt.Errorf("too many updates (%d), max %d per message", len(updates), types.MaxSedaFastUpdatesPerMsg)
			}

			sender := clientCtx.GetFromAddress()
			msg := &types.MsgRelaySedaFastPrices{
				Sender:  sender.String(),
				Updates: updates,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cliflags.AddTxFlagsToCmd(cmd)
	return cmd
}
