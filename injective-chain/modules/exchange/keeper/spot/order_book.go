package spot

import (
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

var _ SpotOrderbook = &SpotLimitOrderbook{}

// GetAllTransientSpotLimitOrderbook returns all transient orderbooks for all spot markets.
func (k SpotKeeper) GetAllTransientSpotLimitOrderbook(ctx sdk.Context) []v2.SpotOrderBook {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllTransientSpotLimitOrderbook")()

	markets := k.GetAllSpotMarkets(ctx)
	orderbook := make([]v2.SpotOrderBook, 0, len(markets)*2)
	for _, market := range markets {
		buyOrders := k.GetAllTransientSpotLimitOrdersByMarketDirection(ctx, market.MarketID(), true)
		orderbook = append(orderbook, v2.SpotOrderBook{
			MarketId:  market.MarketID().Hex(),
			IsBuySide: true,
			Orders:    buyOrders,
		})
		sellOrders := k.GetAllTransientSpotLimitOrdersByMarketDirection(ctx, market.MarketID(), false)
		orderbook = append(orderbook, v2.SpotOrderBook{
			MarketId:  market.MarketID().Hex(),
			IsBuySide: false,
			Orders:    sellOrders,
		})
	}

	return orderbook
}

func NewSpotOrderbookMatchingResults(transientBuyOrders, transientSellOrders []*v2.SpotLimitOrder) *v2.SpotOrderbookMatchingResults {
	orderbookResults := &v2.SpotOrderbookMatchingResults{
		TransientBuyOrderbookFills: &v2.OrderbookFills{
			Orders: transientBuyOrders,
		},
		TransientSellOrderbookFills: &v2.OrderbookFills{
			Orders: transientSellOrders,
		},
	}

	buyFillQuantities := make([]math.LegacyDec, len(transientBuyOrders))
	for idx := range transientBuyOrders {
		buyFillQuantities[idx] = math.LegacyZeroDec()
	}

	sellFillQuantities := make([]math.LegacyDec, len(transientSellOrders))
	for idx := range transientSellOrders {
		sellFillQuantities[idx] = math.LegacyZeroDec()
	}

	orderbookResults.TransientBuyOrderbookFills.FillQuantities = buyFillQuantities
	orderbookResults.TransientSellOrderbookFills.FillQuantities = sellFillQuantities

	return orderbookResults
}

//nolint:revive // ok
type SpotOrderbook interface {
	GetNotional() math.LegacyDec
	GetTotalQuantityFilled() math.LegacyDec
	GetTransientOrderbookFills() *v2.OrderbookFills
	GetRestingOrderbookFills() *v2.OrderbookFills
	Peek(sdk.Context) *v2.PriceLevel
	Fill(sdk.Context, math.LegacyDec)
	Close() error
}

type OrderbookFills struct {
	Orders         []*v2.SpotLimitOrder
	FillQuantities []math.LegacyDec
}

//nolint:revive //ok
type SpotLimitOrderbook struct {
	isBuy         bool
	notional      math.LegacyDec
	totalQuantity math.LegacyDec

	transientOrderbookFills *v2.OrderbookFills
	transientOrderIdx       int

	restingOrderbookFills *v2.OrderbookFills
	restingOrderIterator  storetypes.Iterator

	// pointers to the current OrderbookFills
	currState *v2.OrderbookFills

	k SpotKeeper
}

func NewSpotLimitOrderbook(
	k SpotKeeper,
	iterator storetypes.Iterator,
	transientOrders []*v2.SpotLimitOrder,
	isBuy bool,
) *SpotLimitOrderbook {
	// return early if there are no limit orders in this direction
	if (len(transientOrders) == 0) && !iterator.Valid() {
		iterator.Close()
		return nil
	}

	var transientOrderbookState *v2.OrderbookFills
	if len(transientOrders) == 0 {
		transientOrderbookState = nil
	} else {
		newOrderFillQuantities := make([]math.LegacyDec, len(transientOrders))
		// pre-initialize to zero dec for convenience
		for idx := range newOrderFillQuantities {
			newOrderFillQuantities[idx] = math.LegacyZeroDec()
		}
		transientOrderbookState = &v2.OrderbookFills{
			Orders:         transientOrders,
			FillQuantities: newOrderFillQuantities,
		}
	}

	var restingOrderbookState *v2.OrderbookFills

	if iterator.Valid() {
		restingOrderbookState = &v2.OrderbookFills{
			Orders:         make([]*v2.SpotLimitOrder, 0),
			FillQuantities: make([]math.LegacyDec, 0),
		}
	}

	orderbook := SpotLimitOrderbook{
		isBuy:         isBuy,
		notional:      math.LegacyZeroDec(),
		totalQuantity: math.LegacyZeroDec(),

		transientOrderbookFills: transientOrderbookState,
		transientOrderIdx:       0,
		restingOrderbookFills:   restingOrderbookState,
		restingOrderIterator:    iterator,

		currState: nil,
		k:         k,
	}

	return &orderbook
}

func (b *SpotLimitOrderbook) GetNotional() math.LegacyDec            { return b.notional.Clone() }
func (b *SpotLimitOrderbook) GetTotalQuantityFilled() math.LegacyDec { return b.totalQuantity.Clone() }
func (b *SpotLimitOrderbook) GetTransientOrderbookFills() *v2.OrderbookFills {
	return b.transientOrderbookFills
}
func (b *SpotLimitOrderbook) GetRestingOrderbookFills() *v2.OrderbookFills {
	return b.restingOrderbookFills
}

//nolint:revive // ok
func (b *SpotLimitOrderbook) advanceNewOrder(ctx sdk.Context) {
	if b.currState != nil {
		return
	}

	restingOrder := b.getRestingOrder(ctx)
	transientOrder := b.getTransientOrder(ctx)

	switch {
	case restingOrder != nil && transientOrder != nil:
		// buy orders with higher prices or sell orders with lower prices are prioritized
		if (b.isBuy && restingOrder.OrderInfo.Price.LT(transientOrder.OrderInfo.Price)) ||
			(!b.isBuy && restingOrder.OrderInfo.Price.GT(transientOrder.OrderInfo.Price)) {
			b.currState = b.transientOrderbookFills
		} else {
			b.currState = b.restingOrderbookFills
		}
	case restingOrder != nil && transientOrder == nil:
		b.currState = b.restingOrderbookFills
	case restingOrder == nil && transientOrder != nil:
		b.currState = b.transientOrderbookFills
	default:
	}
}

func (b *SpotLimitOrderbook) Peek(ctx sdk.Context) *v2.PriceLevel {
	defer b.k.Meter(ctx).FuncTiming(&ctx, "SpotLimitOrderbook.Peek")()
	// Sets currState to the orderbook (transientOrderbook or restingOrderbook) with the next best priced order
	b.advanceNewOrder(ctx)

	if b.currState == nil {
		return nil
	}

	idx := b.getCurrIndex()
	order := b.currState.Orders[idx]
	currMatchedQuantity := b.currState.FillQuantities[idx]

	remainingFillableQuantity := order.Fillable.Sub(currMatchedQuantity)

	// Skip orders with zero remaining fillable quantity
	if remainingFillableQuantity.IsZero() {
		b.currState = nil  // Mark current state as exhausted to advance to next order
		return b.Peek(ctx) // Recursively peek next order
	}

	return &v2.PriceLevel{
		Price:    order.OrderInfo.Price,
		Quantity: remainingFillableQuantity,
	}
}

// NOTE: b.currState must NOT be nil!
func (b *SpotLimitOrderbook) getCurrIndex() int {
	var idx int
	// obtain index according to the currState
	if b.currState == b.restingOrderbookFills {
		idx = len(b.restingOrderbookFills.Orders) - 1
	} else {
		idx = b.transientOrderIdx
	}
	return idx
}

func (b *SpotLimitOrderbook) Fill(ctx sdk.Context, fillQuantity math.LegacyDec) {
	defer b.k.Meter(ctx).FuncTiming(&ctx, "SpotLimitOrderbook.Fill")()

	idx := b.getCurrIndex()

	orderCumulativeFillQuantity := b.currState.FillQuantities[idx].Add(fillQuantity)

	b.currState.FillQuantities[idx] = orderCumulativeFillQuantity

	order := b.currState.Orders[idx]
	fillNotional := fillQuantity.Mul(order.OrderInfo.Price)

	b.notional.AddMut(fillNotional)
	b.totalQuantity.AddMut(fillQuantity)

	// if currState is fully filled, set to nil
	if orderCumulativeFillQuantity.Equal(b.currState.Orders[idx].Fillable) {
		b.currState = nil
	}
}

func (b *SpotLimitOrderbook) Close() error {
	return b.restingOrderIterator.Close()
}

func (b *SpotLimitOrderbook) getRestingFillableQuantity() math.LegacyDec {
	idx := len(b.restingOrderbookFills.Orders) - 1
	if idx == -1 {
		return math.LegacyZeroDec()
	}
	return b.restingOrderbookFills.Orders[idx].Fillable.Sub(b.restingOrderbookFills.FillQuantities[idx])
}

func (b *SpotLimitOrderbook) getTransientFillableQuantity() math.LegacyDec {
	idx := b.transientOrderIdx
	return b.transientOrderbookFills.Orders[idx].Fillable.Sub(b.transientOrderbookFills.FillQuantities[idx])
}

func (b *SpotLimitOrderbook) getRestingOrder(ctx sdk.Context) *v2.SpotLimitOrder {
	// if no more orders to iterate + fully filled, return nil
	if !b.restingOrderIterator.Valid() && (b.restingOrderbookFills == nil || b.getRestingFillableQuantity().IsZero()) {
		return nil
	}

	idx := len(b.restingOrderbookFills.Orders) - 1

	// if the current resting order state is fully filled, advance the iterator
	if b.getRestingFillableQuantity().IsZero() {
		// Iteratively skip paused cross-margin orders to avoid recursion proportional
		// to the number of consecutive paused orders at the top of the book.
		for {
			order := b.k.UnmarshalSpotLimitOrder(b.restingOrderIterator.Value())
			b.restingOrderIterator.Next()

			// Check cross-margin emergency pause. During emergency pause, cross-margin
			// orders must not match. Skip the order without adding it to fills so that
			// it remains on the book but does not participate in this block's matching.
			if err := b.k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, order.SubaccountID()); err != nil {
				if !b.restingOrderIterator.Valid() {
					return nil
				}
				continue
			}

			b.restingOrderbookFills.Orders = append(b.restingOrderbookFills.Orders, &order)
			b.restingOrderbookFills.FillQuantities = append(b.restingOrderbookFills.FillQuantities, math.LegacyZeroDec())

			return &order
		}
	}

	return b.restingOrderbookFills.Orders[idx]
}

func (b *SpotLimitOrderbook) getTransientOrder(ctx sdk.Context) *v2.SpotLimitOrder {
	if b.transientOrderbookFills == nil {
		return nil
	}

	if len(b.transientOrderbookFills.Orders) == b.transientOrderIdx {
		return nil
	}

	// Iteratively advance past filled and paused orders to avoid recursion
	// proportional to the number of skipped entries.
	for {
		if b.getTransientFillableQuantity().IsZero() {
			b.transientOrderIdx++
			if len(b.transientOrderbookFills.Orders) == b.transientOrderIdx {
				return nil
			}
			continue
		}

		// Check cross-margin emergency pause. Transient orders from paused subaccounts
		// are normally blocked at placement, but this guards against same-block param changes.
		order := b.transientOrderbookFills.Orders[b.transientOrderIdx]
		if err := b.k.RiskEngine().CheckCrossMarginEmergencyPause(ctx, order.SubaccountID()); err != nil {
			b.transientOrderIdx++
			if len(b.transientOrderbookFills.Orders) == b.transientOrderIdx {
				return nil
			}
			continue
		}

		return order
	}
}

//nolint:revive // ok
type SpotMarketOrderbook struct {
	notional      math.LegacyDec
	totalQuantity math.LegacyDec

	orders         []*v2.SpotMarketOrder
	fillQuantities []math.LegacyDec
	orderIdx       int
}

func NewSpotMarketOrderbook(spotMarketOrders []*v2.SpotMarketOrder) *SpotMarketOrderbook {
	if len(spotMarketOrders) == 0 {
		return nil
	}

	fillQuantities := make([]math.LegacyDec, len(spotMarketOrders))
	for idx := range spotMarketOrders {
		fillQuantities[idx] = math.LegacyZeroDec()
	}

	orderGroup := SpotMarketOrderbook{
		notional:      math.LegacyZeroDec(),
		totalQuantity: math.LegacyZeroDec(),

		orders:         spotMarketOrders,
		fillQuantities: fillQuantities,
		orderIdx:       0,
	}

	return &orderGroup
}

func (b *SpotMarketOrderbook) GetNotional() math.LegacyDec                  { return b.notional.Clone() }
func (b *SpotMarketOrderbook) GetTotalQuantityFilled() math.LegacyDec       { return b.totalQuantity.Clone() }
func (b *SpotMarketOrderbook) GetOrderbookFillQuantities() []math.LegacyDec { return b.fillQuantities }
func (b *SpotMarketOrderbook) Done() bool                                   { return b.orderIdx == len(b.orders) }
func (b *SpotMarketOrderbook) Peek(ctx sdk.Context) *v2.PriceLevel {
	if b.Done() {
		return nil
	}

	if b.fillQuantities[b.orderIdx].Equal(b.orders[b.orderIdx].OrderInfo.Quantity) {
		b.orderIdx++
		return b.Peek(ctx)
	}

	return &v2.PriceLevel{
		Price:    b.orders[b.orderIdx].OrderInfo.Price,
		Quantity: b.orders[b.orderIdx].OrderInfo.Quantity.Sub(b.fillQuantities[b.orderIdx]),
	}
}

func (b *SpotMarketOrderbook) Fill(_ sdk.Context, fillQuantity math.LegacyDec) {
	newFillAmount := b.fillQuantities[b.orderIdx].Add(fillQuantity)

	b.fillQuantities[b.orderIdx] = newFillAmount
	b.notional.AddMut(fillQuantity.Mul(b.orders[b.orderIdx].OrderInfo.Price))
	b.totalQuantity.AddMut(fillQuantity)
}

func (*SpotMarketOrderbook) Close() error {
	// Added for consistency with limit orderbooks interface
	return nil
}
