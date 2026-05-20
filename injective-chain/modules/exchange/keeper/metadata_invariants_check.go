package keeper

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-test/deep"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

type MetadataInvariantCheckConfig struct {
	ShouldCheckSubaccountsBalance bool
}

type MetadataInvariantCheckOption func(*MetadataInvariantCheckConfig)

// IsMetadataInvariantValid should only be used by tests to verify data integrity
func (k *Keeper) IsMetadataInvariantValid(ctx sdk.Context, options ...MetadataInvariantCheckOption) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsMetadataInvariantValid")()

	config := MetadataInvariantCheckConfig{ShouldCheckSubaccountsBalance: true}
	for _, option := range options {
		option(&config)
	}

	m1 := k.getAllSubaccountOrderbookMetadata(ctx)
	m2 := k.getAllSubaccountMetadataFromLimitOrders(ctx)
	m3 := k.getAllSubaccountMetadataFromSubaccountOrders(ctx)

	isValid := true

	if diff := deep.Equal(m1, m2); diff != nil {
		fmt.Println("❌ SubaccountOrderbook metadata doesnt equal metadata derived from limit orders")
		fmt.Println("📢 DIFF: ", diff)
		fmt.Println("1️⃣ SubaccountMetadata", m1)
		fmt.Println("2️⃣ Metadata from LimitOrders", m2)

		k.Logger(ctx).Error("❌ SubaccountOrderbook metadata doesnt equal metadata derived from limit orders")
		k.Logger(ctx).Error("📢 DIFF: ", diff)
		k.Logger(ctx).Error("1️⃣ SubaccountMetadata", m1)
		k.Logger(ctx).Error("2️⃣ Metadata from LimitOrders", m2)
		isValid = false
	}
	if diff := deep.Equal(m2, m3); diff != nil {
		fmt.Println("❌ Metadata derived from limit orders doesnt equal metadata derived from subaccount orders")
		fmt.Println("📢 DIFF: ", diff)
		fmt.Println("2️⃣ Metadata from LimitOrders", m2)
		fmt.Println("3️⃣ Metadata from SubaccountOrders", m3)

		k.Logger(ctx).Error("❌ Metadata derived from limit orders doesnt equal metadata derived from subaccount orders")
		k.Logger(ctx).Error("📢 DIFF: ", diff)
		k.Logger(ctx).Error("2️⃣ Metadata from LimitOrders", m2)
		k.Logger(ctx).Error("3️⃣ Metadata from SubaccountOrders", m3)
		isValid = false
	}
	if diff := deep.Equal(m1, m3); diff != nil {
		fmt.Println("❌ SubaccountOrderbook metadata doesnt equal metadata derived from subaccount orders")
		fmt.Println("📢 DIFF: ", diff)
		fmt.Println("1️⃣ SubaccountMetadata", m1)
		fmt.Println("3️⃣ Metadata from SubaccountOrders", m3)

		k.Logger(ctx).Error("❌ SubaccountOrderbook metadata doesnt equal metadata derived from subaccount orders")
		k.Logger(ctx).Error("📢 DIFF: ", diff)
		k.Logger(ctx).Error("1️⃣ SubaccountMetadata", m1)
		k.Logger(ctx).Error("3️⃣ Metadata from SubaccountOrders", m3)
		isValid = false
	}

	if config.ShouldCheckSubaccountsBalance {
		balances := k.GetAllExchangeBalances(ctx)
		for _, balance := range balances {
			if balance.Deposits.AvailableBalance.IsNegative() {
				_, _ = fmt.Printf(
					"❌ Available %s balance is negative for subaccount %s (%s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.AvailableBalance,
				)
				k.Logger(ctx).Error(fmt.Sprintf(
					"❌ Available %s balance is negative for subaccount %s (%s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.AvailableBalance,
				))
				isValid = false
			}
			if balance.Deposits.TotalBalance.IsNegative() {
				_, _ = fmt.Printf(
					"❌ Total %s balance is negative for subaccount %s (%s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.TotalBalance,
				)
				k.Logger(ctx).Error(fmt.Sprintf(
					"❌ Total %s balance is negative for subaccount %s (%s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.TotalBalance,
				))
				isValid = false
			}
			// Check if available balance is greater than total balance
			// We implement it with tolerance because fuzz tests scenarios could cause an available balance greater
			// than total balance due to a difference in the 18th decimal digit
			availableAndTotalBalanceDifference := balance.Deposits.AvailableBalance.Sub(balance.Deposits.TotalBalance)
			if availableAndTotalBalanceDifference.GT(math.LegacyMustNewDecFromStr("0.000001")) {
				_, _ = fmt.Printf(
					"❌ Available balance is greater than Total balance for %s for subaccount %s (%s > %s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.AvailableBalance,
					balance.Deposits.TotalBalance,
				)
				k.Logger(ctx).Error(fmt.Sprintf(
					"❌ Available balance is greater than Total balance for %s for subaccount %s (%s > %s)",
					balance.Denom,
					balance.SubaccountId,
					balance.Deposits.AvailableBalance,
					balance.Deposits.TotalBalance,
				))
				isValid = false
			}
		}
	}

	isConditionalOrderValid := k.isConditionalOrderInvariantValid(ctx)
	isMarketAggregateVolumeValid := k.IsMarketAggregateVolumeValid(ctx)

	return isValid && isConditionalOrderValid && isMarketAggregateVolumeValid
}

// getAllSubaccountOrderbookMetadata is a helper method only used by tests to verify data integrity
func (k *Keeper) getAllSubaccountOrderbookMetadata(
	ctx sdk.Context,
) map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata {
	defer k.Meter(ctx).FuncTiming(&ctx, "getAllSubaccountOrderbookMetadata")()

	// marketID => isBuy => subaccountID => metadata
	metadatas := make(map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)
	derivativeMarkets := k.GetAllDerivativeMarkets(ctx)
	binaryOptionsMarkets := k.GetAllBinaryOptionsMarkets(ctx)
	markets := make([]v2.DerivativeMarketI, 0, len(derivativeMarkets)+len(binaryOptionsMarkets))

	for _, m := range derivativeMarkets {
		markets = append(markets, m)
	}

	for _, m := range binaryOptionsMarkets {
		markets = append(markets, m)
	}

	for _, market := range markets {
		marketID := market.MarketID()
		k.IterateSubaccountOrderbookMetadataForMarket(
			ctx,
			marketID,
			func(subaccountID common.Hash, isBuy bool, metadata *v2.SubaccountOrderbookMetadata) (stop bool) {
				if metadata.GetOrderSideCount() == 0 {
					return false
				}

				if _, ok := metadatas[marketID]; !ok {
					metadatas[marketID] = make(map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)
				}

				if _, ok := metadatas[marketID][isBuy]; !ok {
					metadatas[marketID][isBuy] = make(map[common.Hash]*v2.SubaccountOrderbookMetadata)
				}

				metadatas[marketID][isBuy][subaccountID] = metadata

				return false
			},
		)
	}

	return metadatas
}

// getAllSubaccountMetadataFromLimitOrders is a helper method only used by tests to verify data integrity
func (k *Keeper) getAllSubaccountMetadataFromLimitOrders(
	ctx sdk.Context,
) map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata {
	defer k.Meter(ctx).FuncTiming(&ctx, "getAllSubaccountMetadataFromLimitOrders")()

	orderbooks := k.GetAllDerivativeAndBinaryOptionsLimitOrderbook(ctx)

	// marketID => isBuy => subaccountID => metadata
	metadatas := make(map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)

	for _, orderbook := range orderbooks {
		marketID := common.HexToHash(orderbook.MarketId)
		isBuy := orderbook.IsBuySide
		m := metadatas[marketID][isBuy]
		for _, order := range orderbook.Orders {
			subaccountID := order.SubaccountID()
			var metadata *v2.SubaccountOrderbookMetadata
			var ok bool

			if _, ok = metadatas[marketID]; !ok {
				metadatas[marketID] = make(map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)
			}

			if _, ok = metadatas[marketID][isBuy]; !ok {
				m = make(map[common.Hash]*v2.SubaccountOrderbookMetadata)
				metadatas[marketID][isBuy] = m
			}

			if metadata, ok = m[subaccountID]; !ok {
				metadata = v2.NewSubaccountOrderbookMetadata()
				m[subaccountID] = metadata
			}
			if order.IsVanilla() {
				metadata.VanillaLimitOrderCount += 1
				metadata.AggregateVanillaQuantity = metadata.AggregateVanillaQuantity.Add(order.Fillable)
			} else {
				metadata.ReduceOnlyLimitOrderCount += 1
				metadata.AggregateReduceOnlyQuantity = metadata.AggregateReduceOnlyQuantity.Add(order.Fillable)
			}
		}
	}

	return metadatas
}

// getAllSubaccountMetadataFromSubaccountOrders is a helper method only used by tests to verify data integrity
func (k *Keeper) getAllSubaccountMetadataFromSubaccountOrders(
	ctx sdk.Context,
) map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata {
	defer k.Meter(ctx).FuncTiming(&ctx, "getAllSubaccountMetadataFromSubaccountOrders")()

	// marketID => isBuy => subaccountID => metadata
	metadatas := make(map[common.Hash]map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)
	k.IterateSubaccountOrders(ctx, func(marketID, subaccountID common.Hash, isBuy bool, order *v2.SubaccountOrder) (stop bool) {
		if _, ok := metadatas[marketID]; !ok {
			metadatas[marketID] = make(map[bool]map[common.Hash]*v2.SubaccountOrderbookMetadata)
		}
		if _, ok := metadatas[marketID][isBuy]; !ok {
			metadatas[marketID][isBuy] = make(map[common.Hash]*v2.SubaccountOrderbookMetadata)
		}

		metadata, ok := metadatas[marketID][isBuy][subaccountID]
		if !ok {
			metadata = v2.NewSubaccountOrderbookMetadata()
			metadatas[marketID][isBuy][subaccountID] = metadata
		}

		if order.IsVanilla() {
			metadata.VanillaLimitOrderCount += 1
			metadata.AggregateVanillaQuantity = metadata.AggregateVanillaQuantity.Add(order.Quantity)
		} else {
			metadata.ReduceOnlyLimitOrderCount += 1
			metadata.AggregateReduceOnlyQuantity = metadata.AggregateReduceOnlyQuantity.Add(order.Quantity)
		}

		return false
	})

	return metadatas
}

func (k *Keeper) verifyConditionalDerivativeLimitOrdersIndexed(
	ctx sdk.Context,
	marketID common.Hash,
	orders []*v2.DerivativeLimitOrder,
	orderKind string,
) bool {
	isValid := true
	for _, order := range orders {
		found, _ := k.GetConditionalDerivativeLimitOrderBySubaccountIDAndHash(ctx, marketID, nil, order.SubaccountID(), order.Hash())
		if found == nil {
			_, _ = fmt.Printf("❌ Conditional %s order %s not found via index for market %s subaccount %s\n",
				orderKind, order.Hash().Hex(), marketID.Hex(), order.SubaccountID().Hex())
			isValid = false
		}
	}
	return isValid
}

func (k *Keeper) verifyConditionalDerivativeMarketOrdersIndexed(
	ctx sdk.Context,
	marketID common.Hash,
	orders []*v2.DerivativeMarketOrder,
	orderKind string,
) bool {
	isValid := true
	for _, order := range orders {
		found, _ := k.GetConditionalDerivativeMarketOrderBySubaccountIDAndHash(ctx, marketID, nil, order.SubaccountID(), order.Hash())
		if found == nil {
			_, _ = fmt.Printf("❌ Conditional %s order %s not found via index for market %s subaccount %s\n",
				orderKind, order.Hash().Hex(), marketID.Hex(), order.SubaccountID().Hex())
			isValid = false
		}
	}
	return isValid
}

// isConditionalOrderInvariantValid verifies that every conditional order in the price-sorted
// store can be looked up via the subaccount index, ensuring the two stores are consistent.
func (k *Keeper) isConditionalOrderInvariantValid(ctx sdk.Context) bool {
	markets := k.GetAllDerivativeMarkets(ctx)
	isValid := true

	for _, market := range markets {
		marketID := market.MarketID()
		orderbook := k.GetAllConditionalDerivativeOrdersUpToMarkPrice(ctx, marketID, nil)
		if orderbook.IsEmpty() {
			continue
		}

		isValid = k.verifyConditionalDerivativeLimitOrdersIndexed(ctx, marketID, orderbook.LimitBuyOrders, "limit buy") && isValid
		isValid = k.verifyConditionalDerivativeLimitOrdersIndexed(ctx, marketID, orderbook.LimitSellOrders, "limit sell") && isValid
		isValid = k.verifyConditionalDerivativeMarketOrdersIndexed(ctx, marketID, orderbook.MarketBuyOrders, "market buy") && isValid
		isValid = k.verifyConditionalDerivativeMarketOrdersIndexed(ctx, marketID, orderbook.MarketSellOrders, "market sell") && isValid
	}

	return isValid
}

// IsMarketAggregateVolumeValid should only be used by tests to verify data integrity
func (k *Keeper) IsMarketAggregateVolumeValid(ctx sdk.Context) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsMarketAggregateVolumeValid")()

	aggregateVolumesList := k.GetAllMarketAggregateVolumes(ctx)
	aggregateVolumes := make(map[common.Hash]v2.VolumeRecord)

	for _, volume := range aggregateVolumesList {
		aggregateVolumes[common.HexToHash(volume.MarketId)] = volume.Volume
	}

	computedVolumes := k.GetAllComputedMarketAggregateVolumes(ctx)

	if diff := deep.Equal(aggregateVolumes, computedVolumes); diff != nil {
		_, _ = fmt.Println("❌ Market aggregated volume doesnt equal volumes derived from subaccount aggregate volumes")
		_, _ = fmt.Println("📢 DIFF: ", diff)
		_, _ = fmt.Println("1️⃣ Market volumes", aggregateVolumes)
		_, _ = fmt.Println("2️⃣ Volumes from subaccount volumes", computedVolumes)

		return false
	}
	return true
}
