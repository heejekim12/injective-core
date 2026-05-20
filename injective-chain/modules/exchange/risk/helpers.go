package risk

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

// ExtractOrderParams extracts direction, quantity, and price from a derivative order.
// Returns false if the order type is unrecognised.
//
//nolint:revive,unparam // function-result-limit: returning 4 values is clearer than a struct for this helper; qty used in tests
func ExtractOrderParams(order DerivativeInitialMarginChecker) (isBuy bool, qty, px math.LegacyDec, ok bool) {
	switch o := order.(type) {
	case *v2.DerivativeLimitOrder:
		return o.IsBuy(), o.Fillable, o.OrderInfo.Price, true
	case *v2.DerivativeMarketOrder:
		return o.IsBuy(), o.OrderInfo.Quantity, o.OrderInfo.Price, true
	default:
		return false, math.LegacyDec{}, math.LegacyDec{}, false
	}
}

// ComputeEntryLoss calculates the entry loss for an order given direction, price, mark price, and quantity.
// Entry loss is the potential loss if the order fills immediately at the mark price.
//
//nolint:revive // flag-parameter: isBuy is a natural parameter for order direction
func ComputeEntryLoss(isBuy bool, orderPrice, markPrice, qty math.LegacyDec) math.LegacyDec {
	if isBuy {
		diff := orderPrice.Sub(markPrice)
		if diff.IsPositive() {
			return diff.Mul(qty)
		}
	} else {
		diff := markPrice.Sub(orderPrice)
		if diff.IsPositive() {
			return diff.Mul(qty)
		}
	}
	return math.LegacyZeroDec()
}

// ComputeAbsWorstExposure calculates the worst-case absolute net exposure for a market.
// This is max(|signedPosQty + buyQty|, |signedPosQty - sellQty|).
func ComputeAbsWorstExposure(signedPosQty, buyQty, sellQty math.LegacyDec) math.LegacyDec {
	return math.LegacyMaxDec(
		signedPosQty.Add(buyQty).Abs(),
		signedPosQty.Sub(sellQty).Abs(),
	)
}

// MarketLockState holds the order quantities and position for a single market,
// used for computing worst-case net exposure in the order-lock requirement.
type MarketLockState struct {
	SignedPosQty math.LegacyDec
	BuyQty       math.LegacyDec
	SellQty      math.LegacyDec
}

// WorstCaseFeeRate returns max(0, max(makerFee, takerFee)), optionally scaled by the atomic
// fee multiplier. This is the worst-case fee rate used for OLR entry-loss and fee-reserve
// calculations throughout the cross-margin model.
// atomicMultiplier should be math.LegacyOneDec() for non-atomic orders.
func WorstCaseFeeRate(
	market v2.DerivativeMarketI,
	atomicMultiplier math.LegacyDec,
) math.LegacyDec {
	rate := math.LegacyMaxDec(math.LegacyZeroDec(), math.LegacyMaxDec(market.GetMakerFeeRate(), market.GetTakerFeeRate()))
	if atomicMultiplier.GT(math.LegacyOneDec()) {
		rate = rate.Mul(atomicMultiplier)
	}
	return rate
}

// BuildMarketLockState constructs the market lock state from position and order metadata.
//
//nolint:revive // cyclomatic: aggregating position + orders requires multiple checks
func BuildMarketLockState(
	ctx sdk.Context,
	deps CrossMarginDeps,
	marketID common.Hash,
	subaccountID common.Hash,
) MarketLockState {
	signedPosQty := math.LegacyZeroDec()
	if pos := deps.Position(ctx, marketID, subaccountID); pos != nil && !pos.Quantity.IsZero() {
		if pos.IsLong {
			signedPosQty = pos.Quantity
		} else {
			signedPosQty = pos.Quantity.Neg()
		}
	}

	metaBuy := deps.SubaccountOrderbookMetadata(ctx, marketID, subaccountID, true)
	metaSell := deps.SubaccountOrderbookMetadata(ctx, marketID, subaccountID, false)

	buyQtyTotal := math.LegacyZeroDec()
	sellQtyTotal := math.LegacyZeroDec()
	if metaBuy != nil {
		buyQtyTotal = metaBuy.AggregateVanillaQuantity
	}
	if metaSell != nil {
		sellQtyTotal = metaSell.AggregateVanillaQuantity
	}

	// Include the transient market order (at most one per direction per block).
	if o := deps.GetTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, true); o != nil && o.IsVanilla() && !o.OrderInfo.Quantity.IsZero() {
		buyQtyTotal = buyQtyTotal.Add(o.OrderInfo.Quantity)
	}
	if o := deps.GetTransientDerivativeMarketOrderForSubaccount(ctx, marketID, subaccountID, false); o != nil && o.IsVanilla() && !o.OrderInfo.Quantity.IsZero() {
		sellQtyTotal = sellQtyTotal.Add(o.OrderInfo.Quantity)
	}

	// NOTE: Transient limit order quantities are already included in
	// SubaccountOrderbookMetadata.AggregateVanillaQuantity (both persistent and transient
	// placement update the same persistent metadata), so no separate iteration is needed here.

	return MarketLockState{
		SignedPosQty: signedPosQty,
		BuyQty:       buyQtyTotal,
		SellQty:      sellQtyTotal,
	}
}

// DecrementLastLookOLROnPool adjusts the stage-local last-look pool state to reflect a cancelled order.
// This is called both from ShouldSkipDerivativeOrderForMarginRequirement (when an inadmissible order
// is pruned) and from DecrementLastLookOLR (when a vanilla order is cancelled by pre-check paths
// outside the admission check, e.g. CheckValidPositionToReduce or emergency pause).
//
//nolint:revive // argument-limit: decomposing order params avoids coupling to a specific order type
func DecrementLastLookOLROnPool(
	ctx sdk.Context,
	deps CrossMarginDeps,
	pool *CrossMarginLastLookPoolState,
	subaccountID common.Hash,
	market v2.DerivativeMarketI,
	markPrice math.LegacyDec,
	isBuy bool,
	qty, px, feeRateWorst math.LegacyDec,
) {
	entryLoss := ComputeEntryLoss(isBuy, px, markPrice, qty)
	feeReserve := feeRateWorst.Mul(px).Mul(qty)

	marketID := market.MarketID()
	mls := pool.MarketLockStateByID[marketID]
	if mls == nil {
		state := BuildMarketLockState(ctx, deps, marketID, subaccountID)
		mls = &CrossMarginLastLookMarketState{
			SignedPosQty: state.SignedPosQty,
			BuyQty:       state.BuyQty,
			SellQty:      state.SellQty,
		}
		pool.MarketLockStateByID[marketID] = mls
	}

	absWorstBefore := ComputeAbsWorstExposure(mls.SignedPosQty, mls.BuyQty, mls.SellQty)

	if isBuy {
		mls.BuyQty = math.LegacyMaxDec(math.LegacyZeroDec(), mls.BuyQty.Sub(qty))
	} else {
		mls.SellQty = math.LegacyMaxDec(math.LegacyZeroDec(), mls.SellQty.Sub(qty))
	}

	absWorstAfter := ComputeAbsWorstExposure(mls.SignedPosQty, mls.BuyQty, mls.SellQty)

	deltaAbsWorst := absWorstBefore.Sub(absWorstAfter)
	if deltaAbsWorst.IsNegative() {
		deltaAbsWorst = math.LegacyZeroDec()
	}

	deltaIMDecrease := market.GetInitialMarginRatio().Mul(markPrice).Mul(deltaAbsWorst)

	quoteDenom := market.GetQuoteDenom()
	pool.OrderLockRequirement = pool.OrderLockRequirement.Sub(deltaIMDecrease).Sub(entryLoss).Sub(feeReserve)
	if pool.OrderLockRequirement.IsNegative() {
		ctx.Logger().Debug("cross-margin OLR floored to zero (precision loss)",
			"subaccount", subaccountID.Hex(),
			"quote_denom", quoteDenom,
			"olr_before_floor", pool.OrderLockRequirement.String(),
			"delta_im_decrease", deltaIMDecrease.String(),
			"entry_loss", entryLoss.String(),
			"fee_reserve", feeReserve.String(),
		)
		pool.OrderLockRequirement = math.LegacyZeroDec()
	}
}
