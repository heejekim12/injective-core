---
sidebar_position: 14
title: Risk System (Cross Margin)
---

# Risk System (Cross Margin)

This design introduces **`cross margin`** as a new risk mode while keeping FBA **parallel and deterministic**. Cross margin is **derivatives-only**, scoped to a **single quote/margin denom pool** (e.g. USDC), and includes **unrealized PnL** in risk checks. It is **not cross-denom -** spot-as-collateral is deferred for portfolio margin phases.

## **Design Philosophy & Trade-offs**

### Hard vs Soft Reservation Models

Cross margin requirements depend on mark prices and can change within a block. Two common enforcement patterns exist:

**Model A: Hard reservation (unified margin)**
- Persist a reserved requirement into spendable collateral (reduces what can be used for other actions).
- Reject collateral-decreasing actions if they would violate the reserved requirement.
- Still needs matching-time validation for mark/state changes, but reduces collateral overcommitment.

**Model B: Soft reservation + matching-time pruning**
- Do not persist `OrderLockRequirement` as a balance lock; admit derivative orders against a snapshot.
- Re-validate at execution ("last-look"); orders that became inadmissible are canceled deterministically.
- Guarantees no inadmissible fills; may show ghost liquidity (orders visible but later canceled on encounter).

**Phase 1 implementation**: cross-margin derivatives use **Model B** (pool-level `OrderLockRequirement` + last-look), to avoid maintaining a continuously-updated reserved amount and to align with EndBlock matching where execution-time validation is required anyway.

### Spot/Derivative Coupling Asymmetry

Cross-margin equity uses `QuoteBalance` derived from `AvailableBalance`, so spot quote-denom holds already reduce derivative admission headroom.

The remaining design choice is whether quote-denom spot actions should also protect existing derivative orders:

- **Variant B1 (symmetric gating)**: reject quote-denom spot collateral-decreases that would make `EquityAdmission < OrderLockRequirement` for that pool.
  - Pros: fewer spot-caused ghost derivative orders; less order-dependent UX.
  - Cons: requires building a cross-pool snapshot during spot admission (adds cost and mark/oracle dependency to the spot path); last-look still required for mark moves.
- **Variant B2 (Phase 1)**: do not check derivative OLR during spot placement.
  - Pros: spot admission stays a local balance/hold check.
  - Cons: spot can create ghost derivative orders, which will be canceled deterministically at matching-time via last-look pruning.

## **Scope & Invariants**

### In Scope

- **Risk modes**: Isolated, cross margin
- **Reservation policy:** full hold ("order locking" at the quote-denom pool level)
- **Collateral scope (cross):** quote/margin denom pool only (e.g. USDC), derivatives-only (perp/expiry as configured; see eligibility)
- **Unrealized PnL:**
    - included in portfolio equity for cross-mode risk checks
    - positive unrealized PnL is haircutted for **admission headroom** (governance parameter, default 50%)
    - `FeesBuffer` is **0** (governance parameter, default 0); fees are covered via order-lock fee reservation, and `FeesBuffer` remains an optional additional safety margin
- **Matching determinism:** enforce cross-mode constraints during EndBlock matching without giving up parallel FBA
- **Liquidation interface:** reuse the existing liquidation message flow (`MsgLiquidatePosition`), with cross-mode semantics (portfolio-level eligibility + cancel-first across the cross pool)

### Out of scope

- Binary options remain isolated-only
- Portfolio margin (borrowing, debt ledger, interest, borrow caps, liquidation waterfalls).
- Cross-denom / multi-asset collateral valuation (e.g. using BTC/ETH spot as collateral for USDC perps).
- Lines of credit (LoC) as an enabled feature.
- Partial/no-hold reservation policies.
- Partial-fill-to-margin (deterministic cancel-on-insufficient-admission rather than partial fills).
- Mark price redesign / begin-of-block mark price snapshotting / oracle pipeline changes (e.g. Slinky-based PreBlock mark updates). Assumes the existing mark/oracle semantics; any price-driven within-block invalidation is handled via deterministic matching-time pruning rather than by eliminating intra-block mark changes.

### **Invariants**

- **Determinism-first**: all multi-subaccount decisions must be canonical and reproducible across validators.
- **Solvency**: cross-mode collateral-decreasing actions must not allow users to fall below maintenance.
- **Bounded work**: no scanning "all subaccounts on every oracle update"; Phase 1 must rely on touched sets + exposure indexes.

### **Oracle dependency**

Cross-margin risk calculations require valid mark prices for all markets in the pool. If a mark price is unavailable (nil or non-positive) for any market with positions or orders, the following operations **will fail**:

- Order placement (admission check cannot be computed)
- Withdrawals / transfers (maintenance check cannot be computed)
- Cross-margin pool snapshot queries

**Rationale**: Using stale or fallback mark prices for risk calculations is dangerous:
- If the actual price is lower than stale, users could open positions they cannot afford
- If the actual price is higher than stale, users could be unfairly liquidated

Phase 1 relies on oracle reliability rather than implementing fallback strategies. Liquidations use the same mark price dependency as the existing isolated margin system.

**Mitigation for operators**: Monitor oracle health and disable cross-margin for affected denoms if sustained oracle outages occur. The graceful wind-down mechanism (remove denom from `CrossMarginEnabledQuoteDenoms`) allows existing users to close positions while blocking new risk.

## Definitions

- All eligible derivative positions and eligible derivative orders that settle in the same quote denom participate in the same **cross pool**. Examples:
    - ✅ Cross-margined together: `BTC/USDC PERP` and `ETH/USDC PERP` positions in the same subaccount.
    - ❌ Not cross-margined together: `DOGE/ETH PERP` and `BTC/USDC PERP` (different quote denom pools).
    - ❌ Not in Phase 1: using BTC/ETH spot balances as collateral for USDC perps (portfolio margin phase).
- One risk profile per subaccount
- Eligibility - Cross-mode must be guarded by explicit chain parameters:
    - **Enabled quote denoms** for cross pools (e.g. `["USDC"]` at launch).
    - **Eligible derivative market types** (e.g. perpetuals only at first, or include expiry futures).

## **Cross Margin Risk Accounting**

### **cross-mode snapshot computed from:**

- `QuoteBalance` (**available** quote-denom collateral, based on `Deposit.AvailableBalance`, net of spot holds and other non-derivative commitments; bank balance is **not** included, including for default subaccounts)
- sum of position margins for eligible derivative positions in the pool
- unrealized PnL from those positions (using mark price)
- open orders, handled via `RESERVATION_POLICY_FULL_HOLD` and pool-level `OrderLockRequirement`

In Phase 1 cross margin, "reserved" capacity for open orders is represented by `OrderLockRequirement` rather than by subtracting per-order holds from `QuoteBalance`. Operationally, the subaccount's admission headroom is `max(0, Equity_admission - OrderLockRequirement)`.
Bank balance exclusion is intentional: default-subaccount bank funds can move via bank-module sends outside exchange risk checks, so only exchange deposits are treated as cross-margin collateral.

> **Note on default subaccounts and cross-margin**: Default subaccounts are *not* excluded from cross-margin. They can have meaningful cross-margin equity from their open positions' margin and unrealised PnL. However, they cannot easily use "free" quote balance as additional collateral because (a) bank balance is excluded from `QuoteBalance` and (b) `SetDepositOrSendToBank` automatically sweeps integer amounts from exchange deposits back to the bank after each settlement. In practice this means default subaccounts have reduced cross-margin headroom compared to non-default subaccounts that retain their full exchange deposit balance as collateral.
Accordingly, `QueryBalanceWithBalanceHolds` reports spot holds and isolated-derivative holds; cross-margin derivative orders do not appear there as per-order `balance_hold`.

**Interaction with spot orders**: See [Spot/Derivative Coupling Asymmetry](#spotderivative-coupling-asymmetry). In Phase 1, derivative admission checks see spot holds (via `QuoteBalance`), but spot placement does not check derivative OLR; quote-denom spot activity can therefore cause matching-time cancellations of derivative orders via last-look pruning.

### **Position margin semantics**

Phase 1 does not redesign Injective's underlying per-position margin accounting. It treats all eligible derivative position margins in the quote-denom pool as contributing to the pool's equity:

- `PositionMarginTotal` is the sum of margins across eligible derivative positions in the pool.
- Increasing position margin remains a collateral-increasing action (it increases pool equity).
- Decreasing position margin is validated in two phases:
  - per-position validation (bankruptcy/position-level safety checks), and
  - pool-level validation when collateral leaves the source cross-margin pool (same constraints as withdrawals/transfers).

**API interpretation of position margin:**

| Risk Mode | Position.Margin | Liquidation Trigger |
|-----------|-----------------|---------------------|
| ISOLATED | Actual collateral reserved for this position | `EffectiveMargin < MaintenanceMargin` (per-position) |
| CROSS | Accounting value (position's attributed share of pool equity) | `PoolEquityLiquidation < PoolMaintenanceMarginTotal` (pool-level) |

For cross-margin positions, the `Margin` field returned by position queries represents an **accounting attribution**, not a hard collateral reservation. The actual collateral backing the position is the entire cross-margin pool's equity, shared across all positions in the pool. Clients should:

1. Check the `risk_mode` field in position query responses to determine margin semantics.
2. For CROSS mode, use `QueryCrossMarginPoolSnapshot` to assess actual pool health and margin headroom.

### **Equity and positive-PnL haircut**

given:

- `uPnL = Σ unrealized_pnl_i(mark_price_i)`
- `uPnL_effective = uPnL` if `uPnL <= 0`
- `uPnL_effective = uPnL * (1 - PositiveUPnLHaircut)` if `uPnL > 0`

then

- `Equity_admission = QuoteBalance + PositionMarginTotal + uPnL_effective - FeesBuffer`
- `Equity_liquidation = QuoteBalance + PositionMarginTotal + uPnL - FeesBuffer` (no positive-PnL haircut for liquidation by default)

Rationale: the positive-uPnL haircut is an **admission-only conservatism** to avoid treating transient positive uPnL as fully available collateral for new risk. Liquidation uses the full uPnL by default to avoid premature liquidations; the haircut does not create additional liquidation buffer.

### **Equity formulas quick reference**

| Metric | Formula | Used for |
|--------|---------|----------|
| `Equity_admission` | `QuoteBalance + PositionMarginTotal + uPnL_effective - FeesBuffer` | Order admission, mode switching |
| `Equity_liquidation` | `QuoteBalance + PositionMarginTotal + uPnL - FeesBuffer` | Maintenance checks, liquidation eligibility |
| `uPnL_effective` | `uPnL * (1 - haircut)` if `uPnL > 0`, else `uPnL` | Admission conservatism |

### **Requirements**

For a cross pool:

- Requirements are computed per eligible market and **summed across markets** (no cross-market offsets in Phase 1).
    - Note: "cross margin" in Phase 1 means **shared collateral**, not portfolio-risk netting (requirements do not offset across markets). Portfolio netting is deferred to portfolio margin phases.

**Positions (current exposure)**

- `IM_positions = Σ (IMR_m * mark_m * |q_m|)`
- `MM_positions = Σ (MMR_m * mark_m * |q_m|)`

where `q_m` is the signed position quantity in market `m` (long positive, short negative).

**Open orders ("order locking")**

To keep the orderbook honest (minimize dead/unexecutable resting orders), Phase 1 marginizes open orders as **worst-case net exposure per market**:

- Let `B_m` be the sum of remaining **fillable buy quantities** of open orders in market `m` for this subaccount
- Let `S_m` be the sum of remaining **fillable sell quantities** of open orders in market `m` for this subaccount
- `absWorst_m = max(|q_m + B_m|, |q_m - S_m|)`
- `IM_with_orders = Σ (IMR_m * mark_m * absWorst_m)`

This automatically nets two-sided quoting within the same market (bid+ask is *not* double-counted), while still summing across markets in the quote-denom pool.

**Reduce-only / close-only orders**

- Reduce-only (and close-only) is enforced at *matching time* against the current position in that market; any quantity that would increase `|q_m|` is **not fillable**.
- If a reduce-only order becomes non-reducing (e.g. the position was closed earlier in the block), it must be cancelled deterministically before it can execute.
- Reduce-only semantics are **per-market**: fills in other markets do not change this market's position `q_m`, so there is no cross-market reduce-only race under parallel per-market matching.

**Conditional orders (stop-loss / take-profit)**

Conditional orders are **not** included in `OrderLockRequirement` at placement time.

- **Placement**: Conditional orders bypass OLR admission; they consume no `OrderLockRequirement`.
- **Trigger-time validation**: At EndBlock trigger, they materialize through the standard order-creation path and run full checks as regular orders:
  1. cross-margin emergency-pause check
  2. market/denom eligibility checks
  3. admission check (`Equity_admission >= OrderLockRequirement_after`) for risk-increasing orders
- **Failure behavior**: if trigger-time validation fails, the conditional order fails to materialize and emits a trigger-failure event.

**Known limitation**: A user can place conditional orders that would pass admission now, then place regular orders that consume all available equity. When the conditional triggers, it may fail admission. The user has no advance warning of this.

**Recommendation for clients**: Before relying on conditional orders for risk management (e.g., stop-losses), check that the subaccount has sufficient headroom by:
1. Using `QueryCrossMarginPoolSnapshot` to assess current equity vs OLR
2. Accounting for the conditional order's potential contribution if it were a regular order
3. Monitoring the `health_factor` field for liquidation proximity

**Execution-price conservatism and fees**

`IM_with_orders` uses `mark_m` by design (stage-fixed, clearing-price independent). However, a fill at an order's execution price can change equity immediately (mark-to-entry uPnL) and will charge fees. To avoid admitting "fillable but instantly under-margined" orders, order locking must also reserve for:

- adverse mark-to-execution uPnL impact (do **not** credit favorable execution vs mark for admission), and
- worst-case execution fees (use a conservative fee rate, e.g. taker fee).

For each open order `i` in market `m` with fillable quantity `qty_i` and a deterministic execution-price bound `px_i`:

- buy: `EntryLoss_i = max(0, (px_i - mark_m) * qty_i)`
- sell: `EntryLoss_i = max(0, (mark_m - px_i) * qty_i)`
- `FeeReserve_i = FeeRateWorst_m * px_i * qty_i`

Then define:

- `OrderLockRequirement = IM_with_orders + Σ EntryLoss_i + Σ FeeReserve_i`

Definitions:

- `FeeRateWorst_m` is the worst-case **total** execution fee rate charged to the trader in market `m` (ignore fee discounts/refunds and ignore negative maker rebates for conservatism). Phase 1 uses taker as the worst case, i.e. `FeeRateWorst_m = max(0, taker_fee_rate_m)`.
- **Atomic order multiplier**: For atomic market orders (orders submitted via `MsgBatchUpdateOrders` with `OrderType` indicating atomic execution), the fee reserve must use `FeeRateWorst_m * AtomicMarketOrderFeeMultiplier_m` to account for the higher execution fee charged for atomic orders. This multiplier is applied per-order during snapshot computation, admission checks, and last-look validation.
- For a vanilla limit order, `px_i` is the limit price.
- For a market/conditional-market order, `px_i` must be an explicit deterministic **worst-price** bound carried on the order. In Phase 1 cross margin, market/conditional-market orders without a worst price bound are rejected.

This keeps the check independent of the eventual FBA clearing price while preventing adverse uPnL/fees surprises.

Example (single market, no positions):

- mark `= 900`, `IMR = 10%`, buy `100 @ 1000`, fees ignored
- `IM_with_orders = 0.1 * 900 * 100 = 9,000`
- `EntryLoss = (1000 - 900) * 100 = 10,000`
- `OrderLockRequirement = 19,000`

Phase 1 does not require portfolio margin modeling beyond aggregating per-market requirements into pool totals.

### **Full-hold for cross margin**

`RESERVATION_POLICY_FULL_HOLD` is used for cross mode as a conservative baseline. Operationally:

- order placement reserves funds up-front (increasing the pool-level reserved `OrderLockRequirement` for open orders)
- cancellations refund the unfilled hold

**Important**: in cross margin, "full-hold" must be applied at the **pool level** (order locking) rather than as a per-order additive hold, otherwise two-sided quoting is double-counted and the book becomes capital-inefficient.

Phase 1 therefore treats "order locking" as *pool-level risk reservation*:

- A risk-increasing order is admitted only if `Equity_admission >= OrderLockRequirement_after` (see requirements above).
- This reserves capacity from the **shared quote-denom pool**, not per-position.

With order locking, parallel per-market matching does **not** need per-market headroom partitioning for correctness; the remaining caveat is mark/uPnL drift, which is handled via deterministic pruning (see below).

## **Admission Rules (User Actions)**

### **Risk-increasing actions (admission)**

Examples:

- placing non-reduce-only derivative orders (or increasing size beyond close-only)
- any action that increases net exposure / initial requirements

Admission rule for cross mode:

- require `Equity_admission >= OrderLockRequirement_after`

### **Collateral-decreasing actions (maintenance)**

Examples:

- withdrawals
- subaccount transfers / external transfers out
- position margin decrease

Maintenance rule for cross mode (standard behavior):

- require `Equity_liquidation >= MM_positions_after`
- additionally, collateral-decreasing actions must not render existing open orders inadmissible; if `Equity_admission_after < OrderLockRequirement_after`, reject the tx (users must cancel risk-increasing orders explicitly)

Position-margin decrease specifics:
- `ValidateDerivativePositionMarginDecrease` always applies position-level checks.
- If decreased margin leaves the source cross-margin pool (for example, transfer to a different subaccount), enforce pool-level checks:
  - `Equity_liquidation_after >= MaintenanceMarginTotal`
  - `Equity_admission_after >= OrderLockRequirement`

## **Deterministic Parallel Matching**

Keeps the existing "parallel per-market" execution model while keeping outcomes deterministic by making cross-margin admission **open-order aware** ("order locking").

### **Key idea: order locking removes cross-market races**

If a cross-mode subaccount satisfies, at the start of a matching stage:

- `Equity_admission >= OrderLockRequirement`

then markets can match in parallel without "double spending" shared headroom.

Intuition: `OrderLockRequirement` is computed from:

- worst-case net position per market (all buys fill vs all sells fill), and
- per-order conservative buffers (`EntryLoss_i`, `FeeReserve_i`) that are computed from stage-fixed marks and deterministic order price bounds.

As fills happen, open orders are consumed, so both the worst-case net position bound and the per-order buffers can only decrease within the stage. As a result, matching cannot increase `OrderLockRequirement` beyond its stage-start value, regardless of market execution order.

### **Stage mark prices (clearing-price independence)**

All cross-margin checks used for order locking and matching-time pruning must use **stage-fixed mark prices** (the same marks used for matching in that stage).

To remove ambiguity:

- **Snapshot timing**: marks are snapshotted once per matching stage (Market-order stage, then Limit-order stage) immediately before launching parallel per-market matching for that stage.
- **Within-stage immutability**: the snapshot is read-only for the duration of that stage's matching. All equity, uPnL, and requirement computations in the stage use the snapshot.
- **Between-stage updates**: if the chain's existing semantics update mark-related inputs deterministically between stages (e.g. after persisting market-order fills), the next stage takes a fresh snapshot.
- **Prepass + on-demand**: matching initializes from stage prepass snapshots for touched subaccounts; if no prepass snapshot exists for a checked subaccount, a snapshot is built on-demand using the same stage mark source.
- **Funding timing**: funding updates are processed in `BeginBlocker` (per `injective-chain/modules/exchange/spec/07_begin_block.md`). Cross-margin snapshots in `EndBlocker` use funding-adjusted positions as per the existing funding settlement semantics.

This keeps risk decisions independent of the eventual FBA clearing price and avoids recursive "remove one order and rerun FBA" loops.

### Endblock flow

```
EndBlock (block n)
├── 1) Trigger conditional MARKET orders
├── 2) Build risk prepass (market-order stage)
├── 3) Run market-order FBA (spot + derivative, parallel)
├── 4) Persist SPOT market-order execution
├── 5) Trigger conditional LIMIT orders
├── 6) Persist DERIVATIVE market-order execution
├── 7) Build risk prepass (limit-order stage)
├── 8) Run limit-order FBA (spot + derivative, parallel)
└── 9) Persist limit-order execution
```

Triggered conditional orders must run the same cross-margin admission ("order locking") checks at materialization time; if an order is no longer admissible, it must be cancelled deterministically before it can participate in matching.

To remove ambiguity:

- Conditional **market** orders are triggered before the market-order stage prepass and therefore participate in that stage's checks.
- Conditional **limit** orders are triggered after spot market-order persistence and before derivative market-order persistence. They see post-spot state but pre-derivative-persistence state; then the limit-stage prepass re-snapshots before limit matching.

**Persistence ordering**

Parallel per-market matching produces a deterministic result per market, but persisting results must also be canonical:

- Persist markets in lexicographic `marketID` order (for the stage), applying each market's internally deterministic ordering (price-time priority) for order updates and cancellations.
- This canonical persistence order ensures deterministic event ordering and deterministic updates to cross-market indexes (e.g. active markets/order markets by subaccount).
- Cross-margin solvency does not depend on the persistence order because order locking is pool-level and does not allocate margin per market; the ordering is strictly for determinism of state transitions and events.

### **Matching-time last look (deterministic pruning)**

Even with order locking at placement time, a resting order can become invalid later (mark price moves, uPnL changes, funding accrues).

Phase 1 does not scan all resting orders on every oracle update. Instead, it guarantees that any order **eligible to execute in this block** is validated (or cancelled) before it can affect the crossing set:

- For a reduce-only / close-only order, re-check reduce-only semantics against the current position in that market; if it is no longer reducing, cancel it deterministically.
- When a risk-increasing order is encountered during the matching scan, require that the subaccount remains admissible at stage marks (`Equity_admission >= OrderLockRequirement`).
- If not, cancel the offending order deterministically and continue the scan.

**Determinism (definition of "step")**

- A "step" is a single deterministic iteration of the matching scan where the engine considers the next candidate buy and sell orders in price-time priority in that market.
- Eligibility is evaluated in a deterministic order: check/cancel the **buy-side** order first, then check/cancel the **sell-side** order. Therefore, if both sides are ineligible on the same candidate match, the buy-side order is cancelled first.
- "Re-run the step" means: after any cancellation, re-peek the next candidate orders (same price level / scan cursor) without advancing the scan past the cancelled order. The loop is bounded because each re-run cancels at least one order.
- Cross-margin admission checks during a stage must be evaluated against the **stage-start snapshot** (stage marks + stage-start positions/open orders) and must not depend on non-canonical cross-market interleavings within the stage. Reduce-only enforcement is evaluated against market-local position updates within the market's matching scan.

**Per-market last-look cache**

To avoid over-cancellation within a single market, a per-market last-look cache tracks the remaining `OrderLockRequirement` as orders are cancelled during orderbook traversal. When an order is cancelled:

1. The order's contribution to OLR (IM component + EntryLoss + FeeReserve) is decremented from the cached remaining OLR.
2. Subsequent orders are checked against the updated remaining OLR.
3. Once `Equity_admission >= remaining_OLR`, further orders in that market pass the last-look check.

This ensures the matcher cancels only the minimum necessary orders within a single market to restore admissibility.

**Note on cross-market over-cancellation**: Each market's matching execution receives its own fresh last-look cache. Therefore, if orders across multiple markets (same quote-denom pool) together cause inadmissibility, the prepass snapshot (computed once at stage start) may result in cancellations in each market independently, even though cancelling orders in just one market might have been sufficient. This is a deliberate trade-off for parallel execution simplicity.

### **Touched set (best-effort cache)**

Touched sources (bounded by current-block activity):

- derivative order placement indicators (market + limit) in this block
- modified positions recorded earlier in this block

When iterating touched subaccounts for stage snapshot computation, canonical ordering is lexicographic by subaccount ID bytes.

## **Invalid Orders Handling**

Cross-mode order validity depends on portfolio equity and portfolio requirements. Therefore resting orders can become invalid without the user placing new transactions.

Phase 1 addresses this without scanning all subaccounts by:

- enforcing order locking on admission/materialization (`Equity_admission >= OrderLockRequirement_after`) so newly accepted orders are collateralized in the quote-denom pool, and
- using **deterministic inline pruning during matching** (last look) when orders are encountered and have become inadmissible due to mark/uPnL drift.

All deterministic cancellations (inadmissible last-look, reduce-only invalidation, conditional materialization failures) should emit the standard order cancellation events so clients can surface the reason without polling.

Importantly, Phase 1 does **not** try to guarantee that the entire resting book is always valid. Instead it guarantees that any order **eligible to execute in this block** is validated (or cancelled) before it can affect the crossing set. Deep resting orders may remain stale until they reach the matching scan.

Pruning heuristics must be deterministic. In Phase 1, the baseline is:

- cancel risk-increasing orders that violate the admission check (`Equity_admission >= OrderLockRequirement`) when they are encountered during the matching scan (orderbook traversal is deterministic by price-time priority),
- if both sides breach on the same candidate match, cancel the buy-side order first (per the deterministic evaluation order), then continue scanning.

Future phases may introduce more nuanced pruning policies (e.g. "newest first" or smallest-notional) if needed.

## **Cross-Margin Liquidation**

### Trigger

- A cross-mode subaccount becomes liquidatable when: `Equity_liquidation < MM_positions`
- Liquidation uses `MsgLiquidateCrossMarginPool`, which atomically closes all positions in the quote-denom pool.
- `MsgLiquidatePosition` rejects cross-margin subaccounts to prevent cherry-picking of profitable positions (which would cause insurance fund losses).

### Waterfall

- Liquidation flow per `MsgLiquidateCrossMarginPool` is:
    1. validate the subaccount has `RiskMode_RISK_MODE_CROSS`, then
    2. discover all pool markets for the given quote denom, then
    3. check pool-level eligibility (`Equity_liquidation < MM_positions`), then
    4. cancel open orders in the cross pool (across eligible markets) to stop further risk and release holds, then
    5. close ALL positions in the pool atomically — if any position cannot be closed (e.g. no liquidity, disabled market), the entire operation fails, then
    6. compute net payout: surplus from profitable positions offsets deficits from unprofitable ones, then
    7. handle payout: positive surplus is split between liquidator reward and insurance fund; negative deficit draws from insurance fund.

Cancellation order must be deterministic. Minimum requirement:

- iterate eligible markets in lexicographic `marketID` order
- within a market, cancel the subaccount's orders using the existing deterministic orderbook traversal/cancellation rules

| Aspect | Scope |
|--------|-------|
| Eligibility check | Pool-level: `Equity_liquidation < MM_positions` across all positions in the quote-denom pool |
| Order cancellation | Pool-level: all orders across the entire pool are cancelled |
| Position liquidation | Pool-level: all positions in the pool are closed atomically via `MsgLiquidateCrossMarginPool` |

**Pool-level atomic liquidation**: `MsgLiquidateCrossMarginPool` closes ALL positions in the quote-denom pool in a single message. Surplus from profitable positions offsets deficits from unprofitable ones before the insurance fund is touched. The operation is atomic — if any position cannot be closed (e.g. no liquidity, disabled market), the entire liquidation fails.

**Per-position liquidation blocked**: `MsgLiquidatePosition` rejects cross-margin subaccounts. This prevents cherry-picking profitable positions, which would cause the insurance fund to subsidise losses that would otherwise be covered by surplus from other positions.

**Discovery**: The `QueryCrossMarginPoolSnapshot` query exposes pool-level risk metrics including `health_factor`. When `health_factor < 1`, the pool is liquidatable via `MsgLiquidateCrossMarginPool`.

**Exceptions**: `MsgOffsetPosition` (admin-only) and `MsgEmergencySettleMarket` bypass the per-position restriction, as these are admin-controlled operations that may need to target specific positions.

## **Interactions with existing mechanisms**

**ADL (auto-deleveraging)** and **expiry settlement** impact cross-margin pools via standard position/PnL state transitions. No special cross-margin handling is required beyond ensuring the pool's exposure indexes and order-lock aggregates are updated consistently with those transitions.

**`MsgOffsetPosition`** (admin-only) and **`MsgEmergencySettleMarket`** share the same internal liquidation path as `MsgLiquidatePosition`. For cross-margin subaccounts:

- **Eligibility** uses pool-level equity (`Equity_liquidation < MM_positions`), not per-position checks.
- **Cancel-first** applies across the entire quote-denom pool, consistent with `MsgLiquidatePosition`.
- **Settlement and position closure** flow through the standard position-delta path; cross-margin exposure indexes (`ActiveDerivativeMarketsBySubaccount`, `ActiveDerivativeOrderMarketsBySubaccount`) are updated via the existing position save/delete mechanics.

No additional cross-margin-specific logic is required for these messages beyond what the shared liquidation path already provides.

## Additions to State

- Exposure indexes to avoid scanning all markets for a subaccount:
    - `ActiveDerivativeMarketsBySubaccount`: tracks markets where the subaccount has a non-zero position.
    - `ActiveDerivativeOrderMarketsBySubaccount`: tracks markets where the subaccount has open derivative orders.
- Phase 1 reuses the existing per-(market, subaccount, direction) `SubaccountOrderbookMetadata` quantity aggregates (e.g. `AggregateVanillaQuantity`) to compute `B_m`/`S_m` for order locking without scanning orderbooks, and should maintain analogous **notional** aggregates (e.g. `Σ price * fillableQty`) to compute `Σ FeeReserve_i` without per-order iteration.
- Phase 1 also reuses the existing transient per-(market, subaccount) order placement indicators (market + limit) to discover markets with transient orders during cross-pool liquidation cancels.
- All indexes must be:
    - updated incrementally on state transitions,
    - bounded by deterministic per-block activity and matching scope; Phase 1 does not mandate a max-markets-per-subaccount limit, but implementations should treat this as a DoS-sensitive surface and may enforce an explicit cap (e.g. 10–20 active order markets per cross pool).

## **Performance and bounded work notes**

- **Avoid scanning all markets**: all per-subaccount computations should iterate only over markets referenced by `ActiveDerivativeMarketsBySubaccount` (positions) and `ActiveDerivativeOrderMarketsBySubaccount` (open orders).
- **Stage snapshot caching**: compute and cache each touched cross-margin subaccount's `(Equity_admission, OrderLockRequirement, IM_positions, MM_positions)` once per stage at stage marks and reuse it for all validations in that stage. No within-stage invalidation is required for the cross-margin admission check because `OrderLockRequirement` is monotonic non-increasing as orders are consumed; only reduce-only enforcement depends on market-local fills within the stage.
- **Order-lock requirement computation**: derive `B_m`/`S_m` from existing `SubaccountOrderbookMetadata` quantity aggregates; compute `Σ FeeReserve_i` from per-(market, subaccount, direction) notional aggregates (`Σ px_i * qty_i`) and `FeeRateWorst_m`; compute `Σ EntryLoss_i` by iterating the subaccount's open orders in its active order markets (bounded by existing protocol limits such as `MaxDerivativeOrderSideCount`), and compute it at most once per stage per touched subaccount.
- **EntryLoss cost bound**: the worst-case per-stage per-subaccount work for `Σ EntryLoss_i` is bounded by the number of open orders in the subaccount's active order markets (at most `2 * MaxDerivativeOrderSideCount` per market for limit/conditional orders). As of current params, `MaxDerivativeOrderSideCount` defaults to 100 (see `injective-chain/modules/exchange/spec/10_params.md`).
- **Tx-time admission cost**: the cross-margin snapshot for a `(subaccount, quoteDenom)` pool is built lazily on first access during DeliverTx and cached in the block-scoped object store. Subsequent derivative order placements for the same pool reuse the cached snapshot and update `OrderLockRequirement` incrementally using the delta of the new order (IM contribution, entry loss, fee reserve). The cache is evicted when an equity-affecting mutation occurs for that subaccount (deposit changes, position changes, order cancellations). Oracle price staleness within a block is accepted and bounded by the EndBlocker last-look safety net.
- **DoS considerations**: cancel-first liquidation and worst-case snapshot building scale with the number of active markets and open orders for the target subaccount. Consider introducing explicit governance caps (e.g. 10–20 active order markets per cross pool) or document reliance on existing protocol limits.

## New API/Events

### **New/updated Tx APIs**

- **MsgUpdateSubaccountRiskProfile**: opt-in to cross margin (subject to eligibility gates and conservative switching rules).

Conservative switching rules:

- **Isolated → Cross**: allowed only if all existing derivative positions and open derivative orders in the subaccount are in **eligible** markets and participate in the same quote-denom cross pool (binary options must not be present). The tx must be rejected if the subaccount would be inadmissible immediately after switching (`Equity_admission < OrderLockRequirement` at current marks).
- **Cross → Isolated**: to avoid complex margin migration semantics in Phase 1, switching back is permitted only if the subaccount has no open derivative positions and no open derivative orders.

### Known limitation: Strict all-or-nothing eligibility for mode switch

**Current behaviour**: `MsgUpdateSubaccountRiskProfile` (isolated → cross) requires ALL derivative positions and orders to be in eligible markets. If a subaccount has even one position in a non-eligible market (binary options, disabled denom, disabled market type), the entire switch is blocked.

**Example**: A user with positions in BTC/USDC PERP and a TRUMP/USDC binary option cannot switch to cross-margin mode until the binary option position is closed.

**Why this design was chosen**:

The alternative "lenient" approach would allow switching with a hybrid state—some positions cross-margined, others isolated. This creates significant complexity:

| Aspect | Strict (current) | Lenient (hybrid) |
|--------|------------------|------------------|
| Mental model | Simple: all derivatives are cross-margined | Complex: need to track which positions participate |
| Liquidation | Pool-level check applies to all positions | Dual checks: isolated for non-eligible, cross for eligible |
| Position margin semantics | Uniform: accounting attribution for all | Mixed: real hold for isolated, attribution for cross |
| API complexity | Single snapshot covers everything | Need both snapshot and per-position APIs for different positions |
| Risk calculation | Straightforward pool-level equity | Must exclude non-eligible positions from pool calculations |

**Consequences for users**:

1. **Must close non-eligible positions first**: Users with binary options, positions in disabled denoms, or positions in disabled market types must close those positions before opting into cross-margin.

2. **All-or-nothing across pools**: If a user has positions in USDC (enabled) and INJ (disabled) quote denom pools, they must close the INJ positions before switching.

3. **Market type gating applies globally**: If `CrossMarginPerpetualEnabled = false` but `CrossMarginExpiryEnabled = true`, users with perpetual positions cannot switch even if they also have expiry positions.

**Future considerations**:

A hybrid approach could be reconsidered if there is strong user demand, but would require:
- Per-position eligibility tracking in state
- Dual liquidation logic (isolated checks + cross checks)
- Clear API semantics for which positions are in which mode
- Careful handling of edge cases (e.g., position becomes ineligible after mode switch due to governance param change)

### **New/updated Query APIs**

- **QuerySubaccountRiskProfile**: returns the effective profile and whether it is the default.
- **QueryCrossMarginPoolSnapshot**: returns the full cross-margin risk snapshot for a (subaccount, quote_denom) pool, including:
  - `quote_balance` - available quote balance in the pool
  - `position_margin_total` - sum of margins across positions
  - `unrealized_pnl` / `unrealized_pnl_effective` - raw and haircutted uPnL
  - `equity_admission` / `equity_liquidation` - equity for admission and liquidation checks
  - `initial_margin_total` / `maintenance_margin_total` - margin requirements for positions
  - `initial_margin_with_orders_total` - IM using worst-case net exposure
  - `entry_loss_total` / `fee_reserve_total` - order-lock components
  - `order_lock_requirement` - total OLR (orders cancelled if equity < OLR)
  - `positive_upnl_haircut_rate` - applied haircut rate
  - `health_factor` - ratio of equity_liquidation to maintenance_margin_total (when < 1, liquidatable; when < 1.5, warning state; nil if no positions)
- **QuerySubaccountPositionInMarket** / **QuerySubaccountEffectivePositionInMarket**: updated to include `risk_mode` field indicating whether the subaccount is in ISOLATED or CROSS mode. For CROSS mode, the position's margin is an accounting value (attributed margin), not a hard collateral reservation; actual collateral is pooled at the quote-denom level. Clients should use `QueryCrossMarginPoolSnapshot` for pool-level health metrics.

### **New parameters (governance)**

- **CrossMarginPositiveUpnlHaircutRate** (default 50%) - applied to **positive** unrealized PnL when computing `Equity_admission` in the cross snapshot
- **CrossMarginFeesBuffer** (default 0) - a fixed buffer **subtracted** from `Equity_admission` and `Equity_liquidation` in the cross snapshot to reserve for fees/slippage
- **CrossMarginEnabledQuoteDenoms** (default `[]` empty) - whitelist of quote denoms that can participate in cross-margin pools. Empty list effectively disables cross-margin globally.
- **CrossMarginPerpetualEnabled** (default `true`) - whether perpetual markets are eligible for cross-margin
- **CrossMarginExpiryEnabled** (default `true`) - whether expiry futures markets are eligible for cross-margin
- **CrossMarginMaxActiveDerivativeMarketsPerPool** (default `100`) - maximum number of active derivative markets per cross-margin pool
- **CrossMarginEmergencyPaused** (default `false`) - emergency pause mode that blocks ALL cross-margin activity including reduce-only orders

### **Governance lifecycle and emergency controls**

**Launching cross-margin (recommended sequence):**

1. Deploy with `CrossMarginEnabledQuoteDenoms = []` (cross-margin disabled)
2. When ready, add denoms to whitelist: `CrossMarginEnabledQuoteDenoms = ["USDC"]`
3. Users can now opt-in to cross-margin mode

**Emergency disable ("graceful wind-down"):**

If cross-margin needs to be disabled after users have opted in (e.g., due to discovered issues):

1. Remove the denom from `CrossMarginEnabledQuoteDenoms` (or set to empty `[]`)
2. **Effects on existing cross-margin users:**
   - ❌ New opt-ins to cross-margin are blocked
   - ❌ New risk-increasing orders are blocked
   - ✅ Reduce-only orders are still allowed (users can close positions)
   - ✅ Liquidations still work (snapshots can still be built)
   - ✅ Users can eventually switch back to isolated mode after closing all positions/orders

**Important:** Once a denom is enabled and users have positions, it cannot be fully "disabled" without allowing graceful wind-down. The design ensures users are never trapped with positions they cannot close.

**Emergency pause (complete halt):**

For critical bugs that require complete cross-margin order flow stoppage (e.g., risk calculation bug, oracle manipulation):

1. Set `CrossMarginEmergencyPaused = true`
2. **Effects on existing cross-margin users:**
   - ❌ New opt-ins to cross-margin are blocked
   - ❌ Risk profile switching involving cross mode is blocked (both isolated → cross and cross → isolated)
   - ❌ ALL orders are blocked (including reduce-only)
   - ✅ Liquidations are still allowed (snapshots can still be built)
   - ✅ Withdrawals are allowed (subject to cross-margin maintenance constraints)

**Rationale:** Emergency pause is a strict incident-containment mode. It freezes all cross-margin state transitions (including opting out) to minimize operational and code-path risk until the incident is resolved.

| Control | Graceful wind-down | Emergency pause |
|---------|-------------------|-----------------|
| New opt-ins | ❌ Blocked | ❌ Blocked |
| Cross-mode profile switching | ✅ Allowed (subject to normal eligibility) | ❌ Blocked |
| Risk-increasing orders | ❌ Blocked | ❌ Blocked |
| Reduce-only orders | ✅ Allowed | ❌ Blocked |
| Liquidations | ✅ Allowed | ✅ Allowed |
| Withdrawals | ✅ Allowed | ✅ Allowed |

**When to use which:**
- **Graceful wind-down**: For planned deprecation or non-critical issues. Users can close positions normally.
- **Emergency pause**: For critical bugs where even reduce-only orders might cause harm. Users must wait for the issue to be resolved.

### **Events**

- Emit explicit events for risk profile updates so clients can observe the effective mode without polling.

## **Phase 1 addresses key concerns**

- **Parallel FBA determinism**: order locking reserves worst-case pooled initial margin plus execution-price/fee buffers up-front (`OrderLockRequirement`), so per-market parallel execution cannot over-consume shared collateral.
- **Invalid orders (ghost orders)**: orders are validated or cancelled **inline during matching** before price formation; deep resting orders may be stale but are not executable this block so we don't care.
- **Bounded work**: no global rescans; Phase 1 relies on incremental updates on order/position transitions plus deterministic cancel-on-encounter.

## Worked Examples

This section provides numerical scenarios that illustrate the key features and limitations of the Phase 1 cross-margin design.

### Example 1: Basic Cross-Margin Benefit (Shared Collateral)

**Scenario**: A trader has positions in two markets with the same quote denom.

| Item | Value |
|------|-------|
| QuoteBalance | 10,000 USDC |
| BTC/USDC position | Long 0.5 BTC @ mark 40,000, margin = 2,000 |
| ETH/USDC position | Short 10 ETH @ mark 2,000, margin = 2,000 |
| BTC uPnL | (40,000 - 39,000 entry) × 0.5 = +500 |
| ETH uPnL | (1,900 entry - 2,000) × 10 = -1,000 |
| IMR (both markets) | 10% |
| MMR (both markets) | 5% |

**Cross-margin calculation**:
- `PositionMarginTotal = 2,000 + 2,000 = 4,000`
- `uPnL = 500 + (-1,000) = -500`
- `Equity_liquidation = 10,000 + 4,000 + (-500) = 13,500`
- `MM_positions = (0.05 × 40,000 × 0.5) + (0.05 × 2,000 × 10) = 1,000 + 1,000 = 2,000`
- Health factor = 13,500 / 2,000 = **6.75** (healthy)

**Isolated-margin comparison**: In isolated mode, each position stands alone:
- BTC position: margin 2,000, uPnL +500 → effective margin 2,500, MM = 1,000 ✓
- ETH position: margin 2,000, uPnL -1,000 → effective margin 1,000, MM = 1,000 ✓ (borderline)

**Key insight**: Cross margin pools the +500 BTC profit to offset the -1,000 ETH loss, giving a single healthy pool rather than one borderline position.

---

### Example 2: Order Locking with Worst-Case Netting

**Scenario**: A market maker quotes both sides in BTC/USDC (no existing position).

| Item | Value |
|------|-------|
| Mark price | 40,000 |
| IMR | 10% |
| Buy order | 0.5 BTC @ 39,900 |
| Sell order | 0.5 BTC @ 40,100 |
| Taker fee rate | 0.1% |

**Naive (incorrect) double-counting**:
- Buy IM = 0.1 × 40,000 × 0.5 = 2,000
- Sell IM = 0.1 × 40,000 × 0.5 = 2,000
- Total = 4,000 ❌

**Correct worst-case netting**:
- If all buys fill: position = +0.5 → `|q + B| = 0.5`
- If all sells fill: position = -0.5 → `|q - S| = 0.5`
- `absWorst = max(0.5, 0.5) = 0.5`
- `IM_with_orders = 0.1 × 40,000 × 0.5 = 2,000`

**EntryLoss and fee reserve**:
- Buy EntryLoss = max(0, 39,900 - 40,000) × 0.5 = 0 (buying below mark)
- Sell EntryLoss = max(0, 40,000 - 40,100) × 0.5 = 0 (selling above mark)
- Buy FeeReserve = 0.001 × 39,900 × 0.5 = 19.95
- Sell FeeReserve = 0.001 × 40,100 × 0.5 = 20.05

**OrderLockRequirement = 2,000 + 0 + 0 + 19.95 + 20.05 = 2,040 USDC**

**Key insight**: Two-sided quoting requires only ~2,040 USDC collateral, not 4,000+. The worst-case netting prevents double-counting.

---

### Example 3: EntryLoss Buffer (Buying Above Mark)

**Scenario**: Aggressive buy order above mark price.

| Item | Value |
|------|-------|
| Mark price | 40,000 |
| IMR | 10% |
| Buy order | 1 BTC @ 42,000 (5% above mark) |
| Taker fee rate | 0.1% |
| Existing position | None |

**OrderLockRequirement calculation**:
- `absWorst = 1` (buy fills → position = 1)
- `IM_with_orders = 0.1 × 40,000 × 1 = 4,000`
- `EntryLoss = max(0, 42,000 - 40,000) × 1 = 2,000`
- `FeeReserve = 0.001 × 42,000 × 1 = 42`

**OrderLockRequirement = 4,000 + 2,000 + 42 = 6,042 USDC**

**Without EntryLoss buffer (hypothetical)**:
- OLR = 4,000 + 42 = 4,042 USDC
- If filled at 42,000: immediate uPnL = (40,000 - 42,000) × 1 = -2,000
- New equity = QuoteBalance + 4,000 (margin) + (-2,000) = QuoteBalance + 2,000
- User would be instantly underwater if they had only 4,042 starting balance

**Key insight**: The EntryLoss buffer prevents "fillable but instantly underwater" orders by reserving for adverse mark-to-execution price difference.

---

### Example 4: Per-Market Last-Look Cache (No Over-Cancellation Within Market)

**Scenario**: Mark price moves against a trader with multiple orders in one market.

| Item | Value |
|------|-------|
| QuoteBalance | 5,000 USDC |
| Mark price | 40,000 (was 38,000 at order placement) |
| IMR | 10% |
| Position | None |
| Orders | 3 buy orders: 0.2 BTC @ 39,000 each |
| Taker fee rate | 0.1% |

**At placement (mark = 38,000)**:
- `absWorst = 0.6`
- `IM_with_orders = 0.1 × 38,000 × 0.6 = 2,280`
- `EntryLoss = max(0, 39,000 - 38,000) × 0.6 = 600`
- `FeeReserve = 0.001 × 39,000 × 0.6 = 23.4`
- `OLR = 2,903.4`, `Equity_admission = 5,000` ✓ Admitted

**At matching (mark = 40,000)**:
- `IM_with_orders = 0.1 × 40,000 × 0.6 = 2,400`
- `EntryLoss = max(0, 39,000 - 40,000) × 0.6 = 0` (now buying below mark)
- `FeeReserve = 23.4`
- `OLR = 2,423.4`, `Equity_admission = 5,000` ✓ Still OK

**Now suppose uPnL from another position dropped equity to 2,000**:
- `Equity_admission = 2,000`
- `OLR = 2,423.4` → Inadmissible

**Last-look cache behaviour**:
1. First order encountered (0.2 BTC): Cancel it
2. Update cached OLR: subtract `(0.1 × 40,000 × 0.2) + 0 + 7.8 = 807.8`
3. New remaining OLR = 2,423.4 - 807.8 = 1,615.6
4. Check: `2,000 >= 1,615.6` ✓ Remaining orders pass

**Result**: Only 1 of 3 orders cancelled (the minimum necessary).

---

### Example 5: Cross-Market Over-Cancellation (Design Limitation)

**Scenario**: Same equity drop, but orders spread across two markets.

| Item | Value |
|------|-------|
| Equity_admission | 2,000 USDC (after uPnL drop) |
| BTC/USDC orders | 2 buy orders: 0.1 BTC @ 39,000 each |
| ETH/USDC orders | 2 buy orders: 0.5 ETH @ 1,900 each |
| BTC mark | 40,000 |
| ETH mark | 2,000 |
| IMR (both) | 10% |

**Total OLR calculation**:
- BTC: `IM = 0.1 × 40,000 × 0.2 = 800`, `FeeReserve ≈ 8` → ~808
- ETH: `IM = 0.1 × 2,000 × 1.0 = 200`, `FeeReserve ≈ 2` → ~202
- Total OLR = ~1,010

**Problem**: Equity (2,000) > OLR (1,010), so the account is actually admissible!

But suppose equity dropped to 900 instead:
- Total OLR = 1,010 > 900 → Inadmissible

**Per-market independent matching** (parallel execution):
- BTC market sees: Equity 900 < BTC OLR component ~808? No, it compares against **total** OLR 1,010
- BTC market cancels orders until its contribution drops to make 900 >= remaining OLR
- ETH market (running in parallel) does the same calculation independently

**Result with parallel execution**:
- BTC market might cancel 1 order (reducing OLR by ~404)
- ETH market might also cancel 1 order (reducing OLR by ~101)
- Total cancellations: 2 orders

**Optimal result** (if sequential):
- Cancel 1 ETH order: OLR drops to ~909
- 900 >= 909? No. Cancel 1 more ETH order: OLR drops to ~808
- 900 >= 808? ✓
- Total cancellations: 2 ETH orders, 0 BTC orders

**Key insight**: Parallel execution may cancel orders in multiple markets when cancelling in just one market would suffice. This is a deliberate trade-off for determinism and parallel performance. In this example, the over-cancellation is modest, but with many markets it could be more significant.

---

### Example 6: Ghost Orders from Spot-After-Derivative

**Scenario**: Derivative orders placed first, then spot orders reduce available balance.

**Step 1 - Place derivative order**:
| Item | Value |
|------|-------|
| QuoteBalance (available) | 5,000 USDC |
| Derivative buy order | 0.1 BTC @ 40,000 |
| OLR | ~440 USDC |
| Equity_admission | 5,000 >= 440 ✓ |

**Step 2 - Place spot buy order**:
| Item | Value |
|------|-------|
| Spot buy order | 100 INJ @ 45 USDC = 4,500 USDC hold |
| Remaining QuoteBalance (available) | 500 USDC |

**Step 3 - At matching time**:
- Cross-margin snapshot: `QuoteBalance = 500` (spot hold already subtracted)
- `Equity_admission = 500`
- `OLR = 440`
- `500 >= 440` ✓ Still admissible (just barely)

**Step 3 (alternative) - Spot order was larger**:
- Spot buy: 110 INJ @ 45 = 4,950 hold
- `QuoteBalance = 50`
- `Equity_admission = 50`
- `OLR = 440`
- `50 < 440` ❌ Derivative order is now a **ghost order**

**Outcome**: The derivative order appears valid in the book but will be cancelled at matching time via last-look.

**Key insight**: Spot orders placed after derivatives can "steal" collateral because spot uses per-order holds while cross-margin derivatives use pool-level OLR. The derivative order becomes a ghost—visible but not executable.

---

### Example 7: Conditional Order Trigger Failure

**Scenario**: User places stop-loss, then uses all equity for regular orders.

**Step 1 - Initial state**:
| Item | Value |
|------|-------|
| QuoteBalance | 10,000 USDC |
| BTC position | Long 0.5 BTC, margin 2,000 |
| OLR | 0 (no open orders) |
| Equity_admission | ~12,000 |

**Step 2 - Place stop-loss** (conditional order):
- Stop-loss sell 0.5 BTC if price < 35,000
- Conditional orders bypass OLR at placement ✓
- OLR remains 0

**Step 3 - Place regular limit orders**:
- Place buy orders totalling OLR = 11,500
- `Equity_admission (12,000) >= OLR (11,500)` ✓ Admitted

**Step 4 - Price drops, stop-loss triggers**:
- Stop-loss attempts to materialise as regular order
- New OLR would be: 11,500 (existing) + ~400 (stop-loss) = 11,900
- `Equity_admission` is now lower due to uPnL loss from price drop
- If `Equity_admission < 11,900` → Stop-loss fails to materialise

**Key insight**: Conditional orders provide no guarantee of execution. Users relying on stop-losses for risk management must maintain sufficient headroom for the conditional to materialise when triggered.

---

## Design Trade-offs

This section summarises the key advantages and disadvantages of the Phase 1 cross-margin design across several dimensions.

### Performance

| Aspect | Assessment |
|--------|------------|
| **Parallel FBA preserved** | ✅ Pro: Per-market matching runs in parallel; order locking prevents cross-market races without sequential coordination |
| **No all-subaccount scans** | ✅ Pro: Risk checks only touch subaccounts with activity in the current block (touched set) |
| **Bounded work** | ✅ Pro: Exposure indexes and existing aggregates (SubaccountOrderbookMetadata) bound iteration to active markets and existing per-side order limits |
| **Stage snapshot cost** | ⚠️ Con: Snapshot computation scales with number of touched cross-margin subaccounts × their active markets; heavy cross-margin activity may increase block time |
| **EntryLoss iteration** | ⚠️ Con: Computing `Σ EntryLoss_i` requires iterating open orders (bounded by `MaxDerivativeOrderSideCount` per market × active markets per subaccount) |

### Capital Efficiency

| Aspect | Assessment |
|--------|------------|
| **Shared collateral** | ✅ Pro: Positions across markets share the same collateral pool; profitable positions offset losing positions |
| **Two-sided netting** | ✅ Pro: Bid/ask quotes in the same market use worst-case net exposure, not double-counted margin |
| **Positive uPnL contributes** | ✅ Pro: Unrealised profits (haircutted at 50% default) can be used for new positions |
| **No cross-market netting** | ❌ Con: Hedged positions (long BTC, short BTC with different expiry) do not offset margin requirements |
| **Full-hold reservation** | ❌ Con: Orders reserve full worst-case margin; no partial-margin or dynamic margining |
| **Conservative fee buffer** | ❌ Con: Always reserves taker fee rate regardless of actual maker/taker status; atomic orders reserve at the higher atomic multiplier rate |
| **Single quote denom** | ❌ Con: Cannot use non-quote assets (e.g., BTC, ETH) as collateral; requires explicit deposit in quote denom |

### User Experience

| Aspect | Assessment |
|--------|------------|
| **No ghost execution** | ✅ Pro: Invalid orders are cancelled before they can execute; users never get unexpected fills |
| **Deterministic outcomes** | ✅ Pro: All cancellations are reproducible and emit events; no non-deterministic races |
| **Emergency controls** | ✅ Pro: Governance can gracefully wind down or emergency pause cross-margin |
| **Ghost resting orders** | ❌ Con: Orders may appear valid in the book but be invalid due to equity changes; cancelled only at matching time |
| **Cross-market over-cancellation** | ❌ Con: When underwater, parallel markets may each cancel orders independently; total cancellations may exceed the minimum necessary |
| **Conditional order uncertainty** | ❌ Con: Stop-loss/take-profit orders may fail to materialise if equity is consumed by regular orders |
| **Spot-after-derivative ghost** | ❌ Con: Spot orders placed after derivatives can reduce available balance, making derivative orders inadmissible |
| **No partial fills to margin** | ❌ Con: If an order would cause inadmissibility, it's fully cancelled rather than partially filled to remaining capacity |

### Guarantees and Non-Guarantees

**What the system guarantees:**

1. **No ghost execution**: An invalid order will never execute. It will be cancelled before it can participate in price formation.
2. **Solvency**: Collateral-decreasing actions (withdrawals, margin decreases) cannot render the account below maintenance margin.
3. **Determinism**: All validators produce identical results; cancellation decisions are reproducible.
4. **Liquidation availability**: Cross-margin liquidations work via the existing `MsgLiquidatePosition` flow with pool-level eligibility.

**What the system does NOT guarantee:**

1. **Order book accuracy**: Resting orders may be invalid (ghost orders) until they reach the matching engine. Query APIs may show orders that will be cancelled.
2. **Minimum cancellation**: When inadmissible, the system cancels orders per-market in parallel; this may exceed the theoretical minimum across all markets.
3. **Conditional order execution**: Stop-loss and take-profit orders may fail to materialise if the account lacks equity at trigger time.
4. **Cross-asset collateral**: Only the quote denom counts as collateral; other assets must be converted first.
5. **Hedge recognition**: Long/short positions in correlated or identical underlyings do not offset margin requirements.

### Oracle Dependency

| Aspect | Assessment |
|--------|------------|
| **Simple implementation** | Pro: No complex fallback logic; relies on oracle reliability |
| **Fail-safe behaviour** | Pro: Operations fail rather than using potentially dangerous stale prices |
| **Oracle outage impact** | Con: If oracle fails for any market in the pool, all pool operations fail |
| **No graceful degradation** | ❌ Con: Cannot trade in healthy markets while one market's oracle is down |

### Comparison with Isolated Margin

| Dimension | Isolated Margin | Cross Margin (Phase 1) |
|-----------|-----------------|------------------------|
| Collateral scope | Per-position | Per-quote-denom pool |
| Liquidation trigger | Per-position | Pool-level |
| Capital efficiency | Lower (no sharing) | Higher (shared collateral) |
| Risk isolation | Higher (losses contained) | Lower (losses can cascade) |
| Complexity | Lower | Higher |
| Ghost orders | Minimal | Possible |
| Oracle dependency | Per-market | All markets in pool |

---

## Known Limitations & Trade-offs

This section consolidates known Phase 1 limitations.

### L1: Cross-market over-cancellation

See [Per-market last-look cache](#per-market-last-look-cache).  
Impact: parallel markets may cancel more orders than the global minimum required to restore admissibility.

### L2: Ghost orders from spot-after-derivative

See [Spot/Derivative Coupling Asymmetry](#spotderivative-coupling-asymmetry).  
Impact: derivative orders can remain visible but be cancelled at matching time.

### L3: Conditional order execution uncertainty

See [Conditional orders (stop-loss / take-profit)](#conditional-orders-stop-loss--take-profit).  
Impact: conditionals can fail to materialize at trigger time if account headroom was consumed meanwhile.

### L4: Liquidation interface asymmetry

See [Known limitation: Asymmetric eligibility vs targeting](#known-limitation-asymmetric-eligibility-vs-targeting).  
Impact: liquidation eligibility is pool-level, but targeting remains position-scoped.

### L5: All-or-nothing mode switch eligibility

See [Known limitation: Strict all-or-nothing eligibility for mode switch](#known-limitation-strict-all-or-nothing-eligibility-for-mode-switch).  
Impact: users must close non-eligible exposure before isolated→cross switch.

---

## Open questions

The items below are policy decisions rather than design gaps. Suggested conservative Phase 1 defaults are included for convenience.

- **Inadmissible accounts policy**: rely on last-look cancel-on-encounter only (no proactive pool-wide cancels outside liquidation); it is sufficient for correctness and avoids additional per-block work.
- **Binary options eligibility**: keep binary options isolated-only in Phase 1.
- **Market/denom gating policy**: start with a single quote-denom pool (e.g. `USDC`) and perpetual markets only.
- **Max active markets cap**: consider adding an explicit cap on active derivative order markets per cross-margin subaccount (e.g. 10–20) to bound worst-case `Σ EntryLoss_i` iteration; with `MaxDerivativeOrderSideCount = 100`, this bounds iteration to ~2,000–4,000 orders per stage.
