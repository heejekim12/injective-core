package subaccount

import (
	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"

	auctiontypes "github.com/InjectiveLabs/injective-core/injective-chain/modules/auction/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/base"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/keeper/events"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

type SubaccountKeeper struct { //nolint:revive // ok
	*base.BaseKeeper

	bank              bankkeeper.Keeper
	account           authkeeper.AccountKeeper
	permissionsKeeper types.PermissionsKeeper
}

func New(
	b *base.BaseKeeper,
	ak authkeeper.AccountKeeper,
	bk bankkeeper.Keeper,
	pk types.PermissionsKeeper,
) *SubaccountKeeper {
	return &SubaccountKeeper{
		BaseKeeper:        b,
		bank:              bk,
		account:           ak,
		permissionsKeeper: pk,
	}
}

func (k *SubaccountKeeper) SetPermissionsKeeper(pk types.PermissionsKeeper) {
	k.permissionsKeeper = pk
}

func (k SubaccountKeeper) IncrementAvailableBalanceOrBank(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementAvailableBalanceOrBank")()

	if amount.IsZero() {
		return
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	deposit.AvailableBalance = deposit.AvailableBalance.Add(amount)
	k.SetDepositOrSendToBank(ctx, subaccountID, denom, *deposit, false)
}

// SetDepositOrSendToBank sets the deposit for a given subaccount and denom. If the subaccount is a default subaccount,
// the positive integer part of the availableDeposit is first sent to the account's bank balance and the deposits are
// set with only the remaining funds.
func (k SubaccountKeeper) SetDepositOrSendToBank(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	deposit v2.Deposit,
	isPreventingBankCharge bool,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetDepositOrSendToBank")()

	amountToSendToBank := deposit.AvailableBalance.TruncateInt()

	// for default subaccounts, if the integer part of the available deposit funds are non-zero, send them to bank
	// otherwise, simply set the deposit to allow for dust accumulation
	shouldSendFundsToBank := amountToSendToBank.IsPositive() && types.IsDefaultSubaccountID(subaccountID)

	if shouldSendFundsToBank {
		// NOTE: AvailableBalance should never be GT TotalBalance, but since in some tests the scenario happened
		// we are adding a check to prevent sending more funds to the bank than the total balance
		truncatedTotalBalance := math.MaxInt(deposit.TotalBalance.TruncateInt(), math.NewInt(0))
		amountToSendToBank := math.MinInt(amountToSendToBank, truncatedTotalBalance)
		accountAddress := types.SubaccountIDToSdkAddress(subaccountID)
		err := k.bank.SendCoinsFromModuleToAccount(
			ctx,
			types.ModuleName, // exchange module
			accountAddress,
			sdk.NewCoins(sdk.NewCoin(denom, amountToSendToBank)),
		)
		if err != nil {
			k.Logger(ctx).Error(
				"CRITICAL: an error occurred when sending default subaccount funds to bank",
				"error", err,
				"subaccountID", subaccountID.Hex(),
				"accountAddress", accountAddress.String(),
				"denom", denom,
				"amountToSendToBank", amountToSendToBank.String(),
			)
		} else {
			deposit.AvailableBalance = deposit.AvailableBalance.Sub(amountToSendToBank.ToLegacyDec())
			deposit.TotalBalance = deposit.TotalBalance.Sub(amountToSendToBank.ToLegacyDec())
		}
	} else {
		shouldChargeFromBank := !isPreventingBankCharge &&
			(deposit.AvailableBalance.IsNegative() || deposit.TotalBalance.IsNegative()) &&
			types.IsDefaultSubaccountID(subaccountID)

		if shouldChargeFromBank {
			amountToCharge := math.LegacyMinDec(deposit.AvailableBalance, deposit.TotalBalance)
			amountToChargeFromBank := amountToCharge.Abs().Ceil().TruncateInt()

			if err := k.ChargeBank(ctx, types.SubaccountIDToSdkAddress(subaccountID), denom, amountToChargeFromBank); err == nil {
				deposit.AvailableBalance = deposit.AvailableBalance.Add(amountToChargeFromBank.ToLegacyDec())
				deposit.TotalBalance = deposit.TotalBalance.Add(amountToChargeFromBank.ToLegacyDec())
			}
		}
	}

	k.SetDeposit(ctx, subaccountID, denom, &deposit)
}

func (k SubaccountKeeper) ChargeBank(
	ctx sdk.Context,
	account sdk.AccAddress,
	denom string,
	amount math.Int,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ChargeBank")()

	if amount.IsZero() {
		return nil
	}

	coin := sdk.NewCoin(denom, amount)

	// ignores "locked" funds in the bank module, but not relevant to us since we don't have locked/vesting bank funds
	balance := k.bank.GetBalance(ctx, account, denom)
	if balance.Amount.LT(amount) {
		return errors.Wrapf(types.ErrInsufficientFunds, "%s is smaller than %s", balance, coin)
	}

	if err := k.bank.SendCoinsFromAccountToModule(ctx, account, types.ModuleName, sdk.NewCoins(coin)); err != nil {
		k.Logger(ctx).Error("bank charge failed", "account", account.String(), "coin", coin.String())
		return errors.Wrap(err, "bank charge failed")
	}

	return nil
}

func (k SubaccountKeeper) UpdateDepositWithDelta(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	depositDelta *types.DepositDelta,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateDepositWithDelta")()

	if depositDelta.IsEmpty() {
		return
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	deposit.AvailableBalance = deposit.AvailableBalance.Add(depositDelta.AvailableBalanceDelta)
	deposit.TotalBalance = deposit.TotalBalance.Add(depositDelta.TotalBalanceDelta)
	k.SetDepositOrSendToBank(ctx, subaccountID, denom, *deposit, false)
}

func (k SubaccountKeeper) HasSufficientFunds(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "HasSufficientFunds")()

	isDefaultSubaccountID := types.IsDefaultSubaccountID(subaccountID)

	if isDefaultSubaccountID {
		bankBalance := k.bank.GetBalance(ctx, types.SubaccountIDToSdkAddress(subaccountID), denom)
		// take the ceiling since we need to round up to the nearest integer due to bank balances being integers
		return bankBalance.Amount.GTE(amount.Ceil().TruncateInt())
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	// usually available balance check is sufficient, but in case of a bug, we check total balance as well
	return deposit.AvailableBalance.GTE(amount) && deposit.TotalBalance.GTE(amount)
}

func (k SubaccountKeeper) ChargeAvailableDeposits(ctx sdk.Context, subaccountID common.Hash, denom string, amount math.LegacyDec) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ChargeAvailableDeposits")()

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	if deposit.IsEmpty() {
		return errors.Wrapf(types.ErrInsufficientDeposit, "Deposits for subaccountID %s asset %s not found", subaccountID.Hex(), denom)
	}

	if deposit.AvailableBalance.LT(amount) {
		return errors.Wrapf(types.ErrInsufficientDeposit, "Insufficient Deposits for subaccountID %s asset %s. Balance decrement %s exceeds Available Balance %s ", subaccountID.Hex(), denom, amount.String(), deposit.AvailableBalance.String())
	}

	// check if account is not blacklisted
	accountAddress := types.SubaccountIDToSdkAddress(subaccountID)
	exchangeAddress := k.account.GetModuleAddress(types.ModuleName)
	if k.permissionsKeeper.IsEnforcedRestrictionsDenom(ctx, denom) {
		if _, err := k.permissionsKeeper.SendRestrictionFn(ctx, accountAddress, exchangeAddress, sdk.NewCoin(denom, amount.TruncateInt())); err != nil {
			return errors.Wrapf(err, "can't charge available deposits from subaccount address %s for %s %s", accountAddress, amount.String(), denom)
		}
	}

	deposit.AvailableBalance = deposit.AvailableBalance.Sub(amount)
	k.SetDeposit(ctx, subaccountID, denom, deposit)
	return nil
}

func (k SubaccountKeeper) chargeBankAndIncrementTotalDeposits(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "chargeBankAndIncrementTotalDeposits")()

	sender := types.SubaccountIDToSdkAddress(subaccountID)
	// round up decimal portion (if exists) - truncation is fine here since we do Ceil first
	intAmount := amount.Ceil().TruncateInt()
	decAmount := intAmount.ToLegacyDec()

	if err := k.ChargeBank(ctx, sender, denom, intAmount); err != nil {
		return err
	}

	// increase available balances by the additional decimal amount charged due to ceil(amount).Int() conversion
	// to ensure that the account does not lose dust, since the account may have been slightly overcharged
	extraChargedAmount := decAmount.Sub(amount)
	k.UpdateDepositWithDelta(ctx, subaccountID, denom, &types.DepositDelta{
		AvailableBalanceDelta: extraChargedAmount,
		TotalBalanceDelta:     decAmount,
	})

	return nil
}

// ChargeAccount transfers the amount from the available balance for non-default subaccounts or the corresponding bank balance if
// the subaccountID is a default subaccount. If bank balances are charged, the total deposits are incremented.
func (k SubaccountKeeper) ChargeAccount(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ChargeAccount")()

	if amount.IsZero() {
		return nil
	}

	if amount.IsNegative() {
		return errors.Wrapf(types.ErrInvalidAmount, "amount charged %s for denom %s must not be negative", amount.String(), denom)
	}

	if types.IsDefaultSubaccountID(subaccountID) {
		return k.chargeBankAndIncrementTotalDeposits(ctx, subaccountID, denom, amount)
	}

	return k.ChargeAvailableDeposits(ctx, subaccountID, denom, amount)
}

func (k SubaccountKeeper) GetSpendableFunds(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
) math.LegacyDec {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSpendableFunds")()

	subaccountDeposits := k.GetDeposit(ctx, subaccountID, denom)
	if !types.IsDefaultSubaccountID(subaccountID) {
		return subaccountDeposits.AvailableBalance
	}

	// combine bankBalance + dust from subaccount deposits to get the total spendable funds
	bankBalance := k.bank.GetBalance(ctx, types.SubaccountIDToSdkAddress(subaccountID), denom)
	return bankBalance.Amount.ToLegacyDec().Add(subaccountDeposits.AvailableBalance)
}

// IncrementSubaccountTradeNonce increments the subaccount's trade nonce and returns the new subaccount trade nonce.
func (k SubaccountKeeper) IncrementSubaccountTradeNonce(
	ctx sdk.Context,
	subaccountID common.Hash,
) *v2.SubaccountTradeNonce {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementSubaccountTradeNonce")()

	subaccountNonce := k.GetSubaccountTradeNonce(ctx, subaccountID)
	subaccountNonce.Nonce++
	k.SetSubaccountTradeNonce(ctx, subaccountID, subaccountNonce)

	return subaccountNonce
}

// UpdateDepositWithDeltaWithoutBankCharge applies a deposit delta to the existing deposit.
func (k SubaccountKeeper) UpdateDepositWithDeltaWithoutBankCharge(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	depositDelta *types.DepositDelta,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateDepositWithDeltaWithoutBankCharge")()

	if depositDelta.IsEmpty() {
		return
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	deposit.AvailableBalance = deposit.AvailableBalance.Add(depositDelta.AvailableBalanceDelta)
	deposit.TotalBalance = deposit.TotalBalance.Add(depositDelta.TotalBalanceDelta)
	k.SetDepositOrSendToBank(ctx, subaccountID, denom, *deposit, true)
}

func (k SubaccountKeeper) UpdateSubaccountOrderbookMetadataFromOrderCancel(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	order *v2.DerivativeLimitOrder,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "UpdateSubaccountOrderbookMetadataFromOrderCancel")()

	metadata := k.GetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, order.IsBuy())
	if order.IsVanilla() {
		metadata.VanillaLimitOrderCount--
		metadata.AggregateVanillaQuantity = metadata.AggregateVanillaQuantity.Sub(order.Fillable)
	} else {
		metadata.ReduceOnlyLimitOrderCount--
		metadata.AggregateReduceOnlyQuantity = metadata.AggregateReduceOnlyQuantity.Sub(order.Fillable)
	}

	k.SetSubaccountOrderbookMetadata(ctx, marketID, subaccountID, order.IsBuy(), metadata)
}

// IncrementDepositWithCoinOrSendToBank increments a given subaccount's deposits by a given coin amount.
func (k SubaccountKeeper) IncrementDepositWithCoinOrSendToBank(
	ctx sdk.Context,
	subaccountID common.Hash,
	coin sdk.Coin,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementDepositWithCoinOrSendToBank")()

	decAmount := coin.Amount.ToLegacyDec()
	k.IncrementDepositOrSendToBank(ctx, subaccountID, coin.Denom, decAmount)
}

// IncrementDepositOrSendToBank increments a given subaccount's deposits by a given dec amount for a given denom
func (k SubaccountKeeper) IncrementDepositOrSendToBank(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementDepositOrSendToBank")()

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	deposit.AvailableBalance = deposit.AvailableBalance.Add(amount)
	deposit.TotalBalance = deposit.TotalBalance.Add(amount)
	k.SetDepositOrSendToBank(ctx, subaccountID, denom, *deposit, false)
}

// IncrementDepositForNonDefaultSubaccount increments a given non-default subaccount's deposits by a given dec amount for a given denom
func (k SubaccountKeeper) IncrementDepositForNonDefaultSubaccount(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "IncrementDepositForNonDefaultSubaccount")()

	if types.IsDefaultSubaccountID(subaccountID) {
		return errors.Wrap(types.ErrBadSubaccountID, subaccountID.Hex())
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)
	deposit.AvailableBalance = deposit.AvailableBalance.Add(amount)
	deposit.TotalBalance = deposit.TotalBalance.Add(amount)

	k.SetDeposit(ctx, subaccountID, denom, deposit)

	return nil
}

// DecrementDeposit decrements a given subaccount's deposits by a given dec amount for a given denom
func (k SubaccountKeeper) DecrementDeposit(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "DecrementDeposit")()

	if amount.IsZero() {
		return nil
	}

	deposit := k.GetDeposit(ctx, subaccountID, denom)

	// usually available balance check is sufficient, but in case of a bug, we check total balance as well
	if deposit.IsEmpty() || deposit.AvailableBalance.LT(amount) || deposit.TotalBalance.LT(amount) {
		return types.ErrInsufficientDeposit
	}
	deposit.AvailableBalance = deposit.AvailableBalance.Sub(amount)
	deposit.TotalBalance = deposit.TotalBalance.Sub(amount)
	k.SetDeposit(ctx, subaccountID, denom, deposit)
	return nil
}

// DecrementDepositOrChargeFromBank decrements a given subaccount's deposits by a given dec amount for a given denom or
// charges the rounded dec amount from the account's bank balance
func (k SubaccountKeeper) DecrementDepositOrChargeFromBank(
	ctx sdk.Context,
	subaccountID common.Hash,
	denom string,
	amount math.LegacyDec,
) (chargeAmount math.LegacyDec, err error) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DecrementDepositOrChargeFromBank")()

	if types.IsDefaultSubaccountID(subaccountID) {
		sender := types.SubaccountIDToSdkAddress(subaccountID)
		// round up decimal portion (if exists) - truncation is fine here since we do Ceil first
		intAmount := amount.Ceil().TruncateInt()
		chargeAmount = intAmount.ToLegacyDec()
		err = k.ChargeBank(ctx, sender, denom, intAmount)
		return chargeAmount, err
	}

	err = k.DecrementDeposit(ctx, subaccountID, denom, amount)
	return amount, err
}

// GetSubaccountOrders returns the subaccount orders for a given marketID and direction sorted by price.
func (k SubaccountKeeper) GetSubaccountOrders(
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
	isStartingIterationFromBestPrice bool,
) []*v2.SubaccountOrderData {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSubaccountOrders")()

	orders := make([]*v2.SubaccountOrderData, 0)
	k.IterateSubaccountOrdersStartingFromOrder(
		ctx,
		marketID,
		subaccountID,
		isBuy,
		isStartingIterationFromBestPrice,
		nil,
		func(order *v2.SubaccountOrder, orderHash common.Hash) (stop bool) {
			orders = append(orders, &v2.SubaccountOrderData{
				Order:     order,
				OrderHash: orderHash.Bytes(),
			})

			return false
		},
	)

	return orders
}

// GetWorstReduceOnlySubaccountOrdersUpToCount returns the first N worst RO subaccount orders for a given marketID
// and direction sorted by price.
func (k SubaccountKeeper) GetWorstReduceOnlySubaccountOrdersUpToCount( //nolint:revive // ok
	ctx sdk.Context,
	marketID common.Hash,
	subaccountID common.Hash,
	isBuy bool,
	totalROCount *uint32,
) (orders []*v2.SubaccountOrderData) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetWorstReduceOnlySubaccountOrdersUpToCount")()

	orders = make([]*v2.SubaccountOrderData, 0)

	remainingROCount := k.GetCachedParams(ctx).MaxDerivativeOrderSideCount
	if totalROCount != nil {
		remainingROCount = *totalROCount
	}

	processOrder := func(order *v2.SubaccountOrder, orderHash common.Hash) (stop bool) {
		if remainingROCount == 0 {
			return true
		}

		if order.IsReduceOnly {
			orders = append(orders, &v2.SubaccountOrderData{
				Order:     order,
				OrderHash: orderHash.Bytes(),
			})

			remainingROCount--
		}

		return false
	}

	k.IterateSubaccountOrdersStartingFromOrder(ctx, marketID, subaccountID, isBuy, false, nil, processOrder)
	return orders
}

func (k SubaccountKeeper) ExecuteWithdraw(ctx sdk.Context, msg *v2.MsgWithdraw) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ExecuteWithdraw")()

	var (
		denom               = msg.Amount.Denom
		amount              = msg.Amount.Amount.ToLegacyDec()
		withdrawDestAddr, _ = sdk.AccAddressFromBech32(msg.Sender)
		subaccountID        = types.MustGetSubaccountIDOrDeriveFromNonce(withdrawDestAddr, msg.SubaccountId)
	)

	// NOTE: Cross-margin maintenance checks are enforced at the message-server layer (before calling ExecuteWithdraw).
	if !k.IsDenomValid(ctx, denom) {
		return sdkerrors.ErrInvalidCoins
	}

	if err := k.DecrementDeposit(ctx, subaccountID, denom, amount); err != nil {
		return errors.Wrap(err, "withdrawal failed")
	}

	if err := k.bank.SendCoinsFromModuleToAccount(ctx, types.ModuleName, withdrawDestAddr, sdk.NewCoins(msg.Amount)); err != nil {
		k.Logger(ctx).Error("subaccount withdrawal failed", "senderAddr", withdrawDestAddr.String(), "coin", msg.Amount.String())
		return errors.Wrap(err, "withdrawal failed")
	}

	events.Emit(ctx, k.BaseKeeper, &v2.EventSubaccountWithdraw{
		SubaccountId: subaccountID.Bytes(),
		DstAddress:   msg.Sender,
		Amount:       msg.Amount,
	})

	return nil
}

func (k SubaccountKeeper) IsDenomValid(ctx sdk.Context, denom string) bool {
	defer k.Meter(ctx).FuncTiming(&ctx, "IsDenomValid")()

	return k.bank.GetSupply(ctx, denom).Amount.IsPositive()
}

//nolint:revive // ok
func (k SubaccountKeeper) WithdrawAllAuctionBalances(ctx sdk.Context, injSendCap math.Int) sdk.Coins {
	defer k.Meter(ctx).FuncTiming(&ctx, "WithdrawAllAuctionBalances")()

	auctionDenomDecimals := k.GetAllAuctionExchangeTransferDenomDecimals(ctx)

	injAuctionSubaccountAmount := math.ZeroInt()

	// collect all balances from auction subaccount deposits and exchange auction fee address
	// the actual sending will be done later, one by one, to handle permission
	// module restrictions
	balancesToTakeFromAuctionSubaccountDeposits := sdk.NewCoins()
	balancesToTakeFromAuctionFeesAddress := sdk.NewCoins()
	for _, auctionDenomDecimal := range auctionDenomDecimals {
		denom := auctionDenomDecimal.Denom

		auctionSubaccountDeposit := k.GetDeposit(ctx, types.AuctionSubaccountID, denom)
		if !auctionSubaccountDeposit.TotalBalance.IsNil() && auctionSubaccountDeposit.TotalBalance.TruncateInt().GT(math.ZeroInt()) {
			amount := auctionSubaccountDeposit.TotalBalance.TruncateInt()
			if denom == chaintypes.InjectiveCoin {
				amount = math.MinInt(amount, injSendCap)
				injAuctionSubaccountAmount = injAuctionSubaccountAmount.Add(amount)
			}
			if amount.IsPositive() {
				coin := sdk.NewCoin(denom, amount)
				balancesToTakeFromAuctionSubaccountDeposits = balancesToTakeFromAuctionSubaccountDeposits.Add(coin)
			}
		}

		auctionFeesAddressBalance := k.bank.GetBalance(ctx, types.ExchangeAuctionFeesAddress, denom)
		if !auctionFeesAddressBalance.IsNil() && auctionFeesAddressBalance.IsPositive() {
			amount := auctionFeesAddressBalance.Amount
			if auctionFeesAddressBalance.Denom == chaintypes.InjectiveCoin {
				remainingCap := math.MaxInt(math.ZeroInt(), injSendCap.Sub(injAuctionSubaccountAmount))
				amount = math.MinInt(amount, remainingCap)
			}
			if amount.IsPositive() {
				coin := sdk.NewCoin(denom, amount)
				balancesToTakeFromAuctionFeesAddress = balancesToTakeFromAuctionFeesAddress.Add(coin)
			}
		}
	}

	// collect all coins that where successfully withdrawn to auction module
	allCoinsSent := sdk.Coins{}

	// Send coins from auction subaccount deposits to auction module one by one
	for _, coin := range balancesToTakeFromAuctionSubaccountDeposits {
		if err := k.bank.SendCoinsFromModuleToModule(
			ctx,
			types.ModuleName,
			auctiontypes.ModuleName,
			sdk.NewCoins(coin),
		); err != nil {
			k.Logger(ctx).Error(
				"CRITICAL: WithdrawAllAuctionBalances failed to transfer from exchange module to auction module",
				"source_module", types.ModuleName,
				"destination_module", auctiontypes.ModuleName,
				"denom", coin.Denom,
				"amount", coin.Amount.String(),
				"error", err,
			)
			continue
		}

		// Now decrement the deposit - if this fails after transfer, we need to roll back
		err := k.DecrementDeposit(ctx, types.AuctionSubaccountID, coin.Denom, coin.Amount.ToLegacyDec())
		if err != nil {
			// CRITICAL: transfer succeeded but deposit decrement failed - this should never happen
			// since we only process denoms that have sufficient balance
			k.Logger(ctx).Error(
				"CRITICAL: WithdrawAllAuctionBalances failed to decrement deposit after transfer",
				"denom", coin.Denom,
				"amount", coin.Amount.String(),
				"error", err,
			)

			// Attempt to roll back the module-to-module transfer to maintain consistency
			rollbackErr := k.bank.SendCoinsFromModuleToModule(
				ctx,
				auctiontypes.ModuleName,
				types.ModuleName,
				sdk.NewCoins(coin),
			)
			if rollbackErr != nil {
				// Rollback failed - state is now inconsistent
				k.Logger(ctx).Error(
					"CRITICAL: failed to rollback transfer after decrement failure - INCONSISTENT STATE",
					"denom", coin.Denom,
					"amount", coin.Amount.String(),
					"rollback_error", rollbackErr,
				)
			} else {
				// Rollback succeeded - state is consistent but the issue should still be investigated
				k.Logger(ctx).Warn(
					"Successfully rolled back transfer after decrement failure",
					"denom", coin.Denom,
					"amount", coin.Amount.String(),
				)
			}

			continue
		}

		allCoinsSent = allCoinsSent.Add(coin)
	}

	// Send coins from exchange auction fee address to auction module one by one
	for _, coin := range balancesToTakeFromAuctionFeesAddress {
		if err := k.bank.SendCoinsFromAccountToModule(
			ctx,
			types.ExchangeAuctionFeesAddress,
			auctiontypes.ModuleName,
			sdk.NewCoins(coin),
		); err != nil {
			k.Logger(ctx).Error(
				"WithdrawAllAuctionBalances failed to transfer from auction fees address to auction module",
				"source_address", types.ExchangeAuctionFeesAddress.String(),
				"destination_module", auctiontypes.ModuleName,
				"denom", coin.Denom,
				"amount", coin.Amount.String(),
				"error", err,
			)
		} else {
			allCoinsSent = allCoinsSent.Add(coin)
		}
	}

	return allCoinsSent
}

// EmitAllTransientDepositUpdates emits the EventDepositUpdate events for all of the deposit updates.
func (k SubaccountKeeper) EmitAllTransientDepositUpdates(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "EmitAllTransientDepositUpdates")()

	subaccountDeposits := make(map[string][]*v2.SubaccountDeposit)
	denoms := make([]string, 0)
	k.IterateTransientDeposits(ctx, func(subaccountID common.Hash, denom string, deposit *v2.Deposit) (stop bool) {
		subaccountDeposit := &v2.SubaccountDeposit{
			SubaccountId: subaccountID.Bytes(),
			Deposit:      deposit,
		}

		if _, ok := subaccountDeposits[denom]; ok {
			subaccountDeposits[denom] = append(subaccountDeposits[denom], subaccountDeposit)
		} else {
			subaccountDeposits[denom] = []*v2.SubaccountDeposit{subaccountDeposit}
			denoms = append(denoms, denom)
		}

		return false
	})

	if len(denoms) > 0 {
		depositUpdates := make([]*v2.DepositUpdate, len(denoms))

		for idx, denom := range denoms {
			depositUpdates[idx] = &v2.DepositUpdate{
				Denom:    denom,
				Deposits: subaccountDeposits[denom],
			}
		}

		events.Emit(ctx, k.BaseKeeper, &v2.EventBatchDepositUpdate{
			DepositUpdates: depositUpdates,
		})
	}
}

// GetAllActivePositionsBySubaccountID returns all active positions for a given subaccountID
func (k SubaccountKeeper) GetAllActivePositionsBySubaccountID(ctx sdk.Context, subaccountID common.Hash) []v2.DerivativePosition {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllActivePositionsBySubaccountID")()

	markets := k.GetAllActiveDerivativeMarkets(ctx)
	positions := make([]v2.DerivativePosition, 0)

	for _, market := range markets {
		marketID := market.MarketID()
		position := k.GetPosition(ctx, marketID, subaccountID)

		if position != nil {
			derivativePosition := v2.DerivativePosition{
				SubaccountId: subaccountID.Hex(),
				MarketId:     marketID.Hex(),
				Position:     position,
			}
			positions = append(positions, derivativePosition)
		}
	}

	return positions
}

func (k SubaccountKeeper) ExecuteDeposit(ctx sdk.Context, msg *v2.MsgDeposit) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "ExecuteDeposit")()

	if !k.IsDenomValid(ctx, msg.Amount.Denom) {
		return sdkerrors.ErrInvalidCoins
	}

	senderAddr, _ := sdk.AccAddressFromBech32(msg.Sender)

	if err := k.bank.SendCoinsFromAccountToModule(ctx, senderAddr, types.ModuleName, sdk.NewCoins(msg.Amount)); err != nil {
		k.Logger(ctx).Error("subaccount deposit failed", "senderAddr", senderAddr.String(), "coin", msg.Amount.String())
		return errors.Wrap(err, "deposit failed")
	}

	var subaccountID common.Hash
	var err error

	subaccountID, err = types.GetSubaccountIDOrDeriveFromNonce(senderAddr, msg.SubaccountId)
	if err != nil {
		// allow deposits to externally owned subaccounts
		subaccountID = common.HexToHash(msg.SubaccountId)
	}

	recipientAddr := types.SubaccountIDToSdkAddress(subaccountID)

	// create new account for recipient if it doesn't exist already
	if !k.account.HasAccount(ctx, recipientAddr) {
		k.account.SetAccount(ctx, k.account.NewAccountWithAddress(ctx, recipientAddr))
	}

	if err := k.IncrementDepositForNonDefaultSubaccount(ctx, subaccountID, msg.Amount.Denom, msg.Amount.Amount.ToLegacyDec()); err != nil {
		return err
	}

	events.Emit(ctx, k.BaseKeeper, &v2.EventSubaccountDeposit{
		SrcAddress:   msg.Sender,
		SubaccountId: subaccountID.Bytes(),
		Amount:       msg.Amount,
	})

	return nil
}

// GetAllPositions returns all positions.
func (k SubaccountKeeper) GetAllPositions(ctx sdk.Context) []v2.DerivativePosition {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllPositions")()

	positions := make([]v2.DerivativePosition, 0)
	appendPosition := func(p *v2.Position, key []byte) (stop bool) {
		subaccountID, marketID := types.GetSubaccountAndMarketIDFromPositionKey(key)
		derivativePosition := v2.DerivativePosition{
			SubaccountId: subaccountID.Hex(),
			MarketId:     marketID.Hex(),
			Position:     p,
		}
		positions = append(positions, derivativePosition)
		return false
	}

	k.IteratePositions(ctx, appendPosition)

	return positions
}
