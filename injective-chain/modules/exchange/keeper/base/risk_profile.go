package base

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types"
	"github.com/InjectiveLabs/injective-core/injective-chain/modules/exchange/types/v2"
)

func DefaultSubaccountRiskProfile() *v2.SubaccountRiskProfile {
	return &v2.SubaccountRiskProfile{
		Mode:              v2.RiskMode_RISK_MODE_ISOLATED,
		ReservationPolicy: v2.ReservationPolicy_RESERVATION_POLICY_FULL_HOLD,
		CreditLineId:      "",
	}
}

func riskProfilesEqual(a, b *v2.SubaccountRiskProfile) bool {
	return a.Mode == b.Mode &&
		a.ReservationPolicy == b.ReservationPolicy &&
		a.CreditLineId == b.CreditLineId
}

// NormalizeRiskProfile validates and normalizes a stored risk profile.
// Returns (profile, false) if the profile uses unsupported features (credit lines, non-FULL_HOLD policy).
func NormalizeRiskProfile(profile *v2.SubaccountRiskProfile) (normalized *v2.SubaccountRiskProfile, ok bool) {
	if profile == nil {
		return DefaultSubaccountRiskProfile(), true
	}

	normalized = &v2.SubaccountRiskProfile{
		Mode:              profile.Mode,
		ReservationPolicy: profile.ReservationPolicy,
		CreditLineId:      profile.CreditLineId,
	}

	// Default UNSPECIFIED values for backward compatibility (and to prevent storing invalid enums).
	if normalized.Mode == v2.RiskMode_RISK_MODE_UNSPECIFIED {
		normalized.Mode = v2.RiskMode_RISK_MODE_ISOLATED
	}
	if normalized.ReservationPolicy == v2.ReservationPolicy_RESERVATION_POLICY_UNSPECIFIED {
		normalized.ReservationPolicy = v2.ReservationPolicy_RESERVATION_POLICY_FULL_HOLD
	}

	// Reject features not yet implemented (fail closed for unknown enum values too).
	if normalized.Mode != v2.RiskMode_RISK_MODE_ISOLATED && normalized.Mode != v2.RiskMode_RISK_MODE_CROSS {
		return DefaultSubaccountRiskProfile(), false
	}
	if normalized.ReservationPolicy != v2.ReservationPolicy_RESERVATION_POLICY_FULL_HOLD {
		return DefaultSubaccountRiskProfile(), false
	}
	if normalized.CreditLineId != "" {
		return DefaultSubaccountRiskProfile(), false
	}

	return normalized, true
}

func (k *BaseKeeper) GetSubaccountRiskProfile(ctx sdk.Context, subaccountID common.Hash) (*v2.SubaccountRiskProfile, bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetSubaccountRiskProfile")()

	store := k.getStore(ctx)
	profileStore := prefix.NewStore(store, types.SubaccountRiskProfilePrefix)

	bz := profileStore.Get(subaccountID.Bytes())
	if bz == nil {
		return nil, false
	}

	var profile v2.SubaccountRiskProfile
	k.cdc.MustUnmarshal(bz, &profile)

	return &profile, true
}

func (k *BaseKeeper) SetSubaccountRiskProfile(ctx sdk.Context, subaccountID common.Hash, profile *v2.SubaccountRiskProfile) error {
	defer k.Meter(ctx).FuncTiming(&ctx, "SetSubaccountRiskProfile")()

	if profile != nil && profile.Mode == v2.RiskMode_RISK_MODE_CROSS && types.IsDefaultSubaccountID(subaccountID) {
		return types.ErrInvalidState.Wrap("default subaccounts cannot use cross-margin mode")
	}

	normalized, ok := NormalizeRiskProfile(profile)
	if !ok {
		return types.ErrFeatureDisabled.Wrap("risk profile not supported")
	}

	// No-op if the effective profile already equals the requested profile.
	// This covers both: (a) stored record matches, and (b) no record exists and the
	// requested profile is the module default — avoids writing a redundant record that
	// would change is_default from true to false without changing behaviour.
	stored, _ := k.GetSubaccountRiskProfile(ctx, subaccountID)
	effectiveNormalized, _ := NormalizeRiskProfile(stored) // nil stored → default
	if riskProfilesEqual(effectiveNormalized, normalized) {
		return nil
	}

	isDefault := riskProfilesEqual(normalized, DefaultSubaccountRiskProfile())

	store := k.getStore(ctx)
	profileStore := prefix.NewStore(store, types.SubaccountRiskProfilePrefix)

	if isDefault {
		// Reverting to the module default: delete the record so the subaccount is
		// represented by the implicit default, keeping is_default=true and avoiding
		// redundant genesis export entries.
		profileStore.Delete(subaccountID.Bytes())
	} else {
		bz := k.cdc.MustMarshal(normalized)
		profileStore.Set(subaccountID.Bytes(), bz)
	}

	return nil
}

func (k *BaseKeeper) DeleteSubaccountRiskProfile(ctx sdk.Context, subaccountID common.Hash) {
	defer k.Meter(ctx).FuncTiming(&ctx, "DeleteSubaccountRiskProfile")()

	store := k.getStore(ctx)
	profileStore := prefix.NewStore(store, types.SubaccountRiskProfilePrefix)
	profileStore.Delete(subaccountID.Bytes())
}

// GetAllSubaccountRiskProfiles returns all non-default subaccount risk profiles for genesis export.
func (k *BaseKeeper) GetAllSubaccountRiskProfiles(ctx sdk.Context) []*v2.SubaccountRiskProfileRecord {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllSubaccountRiskProfiles")()

	store := k.getStore(ctx)
	profileStore := prefix.NewStore(store, types.SubaccountRiskProfilePrefix)

	records := make([]*v2.SubaccountRiskProfileRecord, 0)

	iterateSafe(profileStore.Iterator(nil, nil), func(key, value []byte) bool {
		subaccountID := common.BytesToHash(key[:common.HashLength])
		var profile v2.SubaccountRiskProfile
		k.cdc.MustUnmarshal(value, &profile)
		records = append(records, &v2.SubaccountRiskProfileRecord{
			SubaccountId: subaccountID.Hex(),
			RiskProfile:  profile,
		})
		return false
	})

	return records
}

func (k *BaseKeeper) GetEffectiveSubaccountRiskProfile(ctx sdk.Context, subaccountID common.Hash) (profile *v2.SubaccountRiskProfile, isDefault bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetEffectiveSubaccountRiskProfile")()

	stored, found := k.GetSubaccountRiskProfile(ctx, subaccountID)
	if !found {
		return DefaultSubaccountRiskProfile(), true
	}

	normalized, ok := NormalizeRiskProfile(stored)
	if !ok {
		return DefaultSubaccountRiskProfile(), false
	}
	return normalized, false
}
