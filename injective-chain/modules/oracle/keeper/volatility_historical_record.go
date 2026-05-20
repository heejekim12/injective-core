package keeper

import (
	"cmp"
	"encoding/binary"
	"slices"

	"cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/InjectiveLabs/injective-core/injective-chain/modules/oracle/types"
	chaintypes "github.com/InjectiveLabs/injective-core/injective-chain/types"
)

func (k *Keeper) AppendPriceRecord(ctx sdk.Context, oracleType types.OracleType, symbol string, priceRecord *types.PriceRecord) {
	defer k.Meter(ctx).FuncTiming(&ctx, "AppendPriceRecord")()

	store := k.getStore(ctx)
	key := types.GetSymbolHistoricalPriceRecordKey(oracleType, symbol, priceRecord.Timestamp)
	bz, err := priceRecord.Price.Marshal()
	if err != nil {
		return
	}
	store.Set(key, bz)
}

func (k *Keeper) CleanupHistoricalPriceRecords(ctx sdk.Context) {
	defer k.Meter(ctx).FuncTiming(&ctx, "CleanupHistoricalPriceRecords")()

	store := k.getStore(ctx)
	before := ctx.BlockTime().Unix() - types.MaxHistoricalPriceRecordAge
	endTsBytes := sdk.Uint64ToBigEndian(uint64(before))

	cursor := store.Get(types.CleanupCursorKey)
	histStore := prefix.NewStore(store, types.SymbolHistoricalPriceRecordsPrefix)

	type symKey struct {
		ot  types.OracleType
		sym string
	}
	var batch []symKey
	var nextCursor []byte
	var curOt types.OracleType
	var curSym string
	seenAny := false

	chaintypes.IterateKeysSafe(histStore.Iterator(cursor, nil), func(rawKey []byte) bool {
		fullKey := append(append([]byte{}, types.SymbolHistoricalPriceRecordsPrefix...), rawKey...)
		ot, sym, _, ok := types.ParseSymbolHistoricalPriceRecordKey(fullKey)
		if !ok {
			return false
		}
		if !seenAny || ot != curOt || sym != curSym {
			if seenAny {
				batch = append(batch, symKey{ot: curOt, sym: curSym})
				if len(batch) >= types.MaxSymbolsPerCleanupRound {
					nextCursor = rawKey
					return true
				}
			}
			curOt, curSym, seenAny = ot, sym, true
		}
		return false
	})

	if seenAny && nextCursor == nil {
		batch = append(batch, symKey{ot: curOt, sym: curSym})
	}

	for _, sk := range batch {
		prefixKey := types.GetSymbolHistoricalPriceRecordPrefix(sk.ot, sk.sym)
		sub := prefix.NewStore(store, prefixKey)
		var toDelete [][]byte
		chaintypes.IterateKeysSafe(sub.Iterator(nil, endTsBytes), func(tsKey []byte) bool {
			toDelete = append(toDelete, tsKey)
			return false
		})
		for _, dk := range toDelete {
			sub.Delete(dk)
		}
	}

	if nextCursor != nil {
		store.Set(types.CleanupCursorKey, nextCursor)
	} else {
		store.Delete(types.CleanupCursorKey)
	}
}

func (k *Keeper) loadAllHistoricalPriceRecordsForSymbol(
	ctx sdk.Context,
	oracleType types.OracleType,
	symbol string,
) []*types.PriceRecord {
	store := k.getStore(ctx)
	prefixKey := types.GetSymbolHistoricalPriceRecordPrefix(oracleType, symbol)
	sub := prefix.NewStore(store, prefixKey)

	var out []*types.PriceRecord
	chaintypes.IterateSafe(sub.Iterator(nil, nil), func(k, v []byte) bool {
		if len(k) != 8 {
			return false
		}
		ts := int64(binary.BigEndian.Uint64(k))
		var price math.LegacyDec
		if err := price.Unmarshal(v); err != nil {
			return false
		}
		out = append(out, &types.PriceRecord{Timestamp: ts, Price: price})
		return false
	})
	return out
}

// GetMixedHistoricalPriceRecords returns the merged historical prices from two separate price feeds.
// Example:
//
//	Time          --- 1 ---- 2 ---- 3 ---- 4
//	BTC Price     --- 6 ---- 8 ---- 4 ---- 5
//	ETH Price     --- 3 ----   ---- 4 ---- 2
//	BTC/ETH Price --- 2 -- 2.6667 -- 1 ---- 2.5
//	Price is 2.6667 since 8/3 = 2.666... since ETH price = 3 from the last timestamp
func (k *Keeper) GetMixedHistoricalPriceRecords(
	ctx sdk.Context,
	baseOracleType, quoteOracleType types.OracleType,
	baseSymbol, quoteSymbol string,
	from int64,
) (mixed *types.PriceRecords, ok bool) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetMixedHistoricalPriceRecords")()

	basePriceRecords := k.loadAllHistoricalPriceRecordsForSymbol(ctx, baseOracleType, baseSymbol)
	quotePriceRecords := k.loadAllHistoricalPriceRecordsForSymbol(ctx, quoteOracleType, quoteSymbol)

	if len(basePriceRecords) == 0 || len(quotePriceRecords) == 0 {
		return nil, false
	}

	mixed = &types.PriceRecords{
		LatestPriceRecords: make([]*types.PriceRecord, 0, len(basePriceRecords)+len(quotePriceRecords)),
	}

	basePriceTimestampMap := make(map[int64]math.LegacyDec, len(basePriceRecords))
	quotePriceTimestampMap := make(map[int64]math.LegacyDec, len(quotePriceRecords))
	uniqueTimestamps := make([]int64, 0, len(basePriceRecords)+len(quotePriceRecords))

	// Assuming price records are sorted by timestamp, we go backwards until we meet first timestamp < from, add it and break.
	// We add it to be able to find prevPrice for first record that satisfies timestamp condition
	for i := len(basePriceRecords) - 1; i >= 0; i-- {
		r := basePriceRecords[i]

		if _, ok := basePriceTimestampMap[r.Timestamp]; ok {
			continue
		} else {
			basePriceTimestampMap[r.Timestamp] = r.Price
			uniqueTimestamps = append(uniqueTimestamps, r.Timestamp)
		}
		if r.Timestamp < from {
			break
		}
	}

	for i := len(quotePriceRecords) - 1; i >= 0; i-- {
		r := quotePriceRecords[i]

		_, baseExists := basePriceTimestampMap[r.Timestamp]
		_, quoteExists := quotePriceTimestampMap[r.Timestamp]

		if !quoteExists {
			quotePriceTimestampMap[r.Timestamp] = r.Price
			if !baseExists {
				uniqueTimestamps = append(uniqueTimestamps, r.Timestamp)
			}
		}
		if r.Timestamp < from {
			break
		}
	}

	findPrevPrice := func(prices map[int64]math.LegacyDec, idx int) (math.LegacyDec, bool) {
		var offset int
		for idx+offset > 0 {
			offset--

			p, exists := prices[uniqueTimestamps[idx+offset]]
			if exists {
				// fetched some price at past time
				return p, true
			}
		}

		return math.LegacyZeroDec(), false
	}

	// NOTE: uniqueTimestamps contains reverse sorted mixed timeline from both records
	slices.SortFunc(uniqueTimestamps, cmp.Compare)

	for idx, t0 := range uniqueTimestamps {
		basePrice, baseExists := basePriceTimestampMap[t0]
		quotePrice, quoteExists := quotePriceTimestampMap[t0]

		var price math.LegacyDec

		switch {
		case baseExists && quoteExists:
			// both prices present in the same time instant
			price = basePrice.Quo(quotePrice)
		case !baseExists:
			prevBase, ok := findPrevPrice(basePriceTimestampMap, idx)
			if !ok {
				// unable to find previous base price, maybe there will be some later
				continue
			}

			// found some previous base price
			price = prevBase.Quo(quotePrice)
		case !quoteExists:
			prevQuote, ok := findPrevPrice(quotePriceTimestampMap, idx)
			if !ok {
				// unable to find previous quote price, maybe there will be some later
				continue
			}

			// found some previous quote price
			price = basePrice.Quo(prevQuote)
		}

		mixed.LatestPriceRecords = append(mixed.LatestPriceRecords, &types.PriceRecord{
			Timestamp: t0,
			Price:     price,
		})
	}
	// as we have one timestamp before from, we should filter that
	mixed.LatestPriceRecords, _ = filterHistoricalPriceRecords(mixed.LatestPriceRecords, from)

	if len(mixed.LatestPriceRecords) == 0 {
		return nil, false
	}
	return mixed, true
}

// GetHistoricalPriceRecords returns the historical price records for an oracleType + symbol starting from the `from` time
func (k *Keeper) GetHistoricalPriceRecords(
	ctx sdk.Context,
	oracleType types.OracleType,
	symbol string,
	from int64,
) *types.PriceRecords {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetHistoricalPriceRecords")()

	entry := &types.PriceRecords{
		Oracle:   oracleType,
		SymbolId: symbol,
	}

	store := k.getStore(ctx)
	prefixKey := types.GetSymbolHistoricalPriceRecordPrefix(oracleType, symbol)
	sub := prefix.NewStore(store, prefixKey)
	var startKey []byte
	if from > 0 {
		startKey = sdk.Uint64ToBigEndian(uint64(from))
	}

	chaintypes.IterateSafe(sub.Iterator(startKey, nil), func(k, v []byte) bool {
		if len(k) != 8 {
			return false
		}
		ts := int64(binary.BigEndian.Uint64(k))
		var price math.LegacyDec
		if err := price.Unmarshal(v); err != nil {
			return false
		}
		entry.LatestPriceRecords = append(entry.LatestPriceRecords, &types.PriceRecord{
			Timestamp: ts,
			Price:     price,
		})
		return false
	})

	return entry
}

func (k *Keeper) GetAllHistoricalPriceRecords(ctx sdk.Context) []*types.PriceRecords {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetAllHistoricalPriceRecords")()

	store := k.getStore(ctx)
	histStore := prefix.NewStore(store, types.SymbolHistoricalPriceRecordsPrefix)

	type groupKey struct {
		ot  types.OracleType
		sym string
	}
	recordsByGroup := make(map[groupKey][]*types.PriceRecord)

	chaintypes.IterateSafe(histStore.Iterator(nil, nil), func(k, v []byte) bool {
		fullKey := append(append([]byte{}, types.SymbolHistoricalPriceRecordsPrefix...), k...)
		ot, sym, ts, ok := types.ParseSymbolHistoricalPriceRecordKey(fullKey)
		if !ok {
			return false
		}
		var price math.LegacyDec
		if err := price.Unmarshal(v); err != nil {
			return false
		}
		gk := groupKey{ot: ot, sym: sym}
		recordsByGroup[gk] = append(recordsByGroup[gk], &types.PriceRecord{
			Timestamp: ts,
			Price:     price,
		})
		return false
	})

	out := make([]*types.PriceRecords, 0, len(recordsByGroup))
	for gk, recs := range recordsByGroup {
		out = append(out, &types.PriceRecords{
			Oracle:             gk.ot,
			SymbolId:           gk.sym,
			LatestPriceRecords: recs,
		})
	}

	slices.SortStableFunc(out, func(a, b *types.PriceRecords) int {
		if a.Oracle != b.Oracle {
			return cmp.Compare(a.Oracle, b.Oracle)
		}
		return cmp.Compare(a.SymbolId, b.SymbolId)
	})

	return out
}

func filterHistoricalPriceRecords(
	records []*types.PriceRecord,
	from int64,
) (filteredRecords []*types.PriceRecord, omitted bool) {
	offsetIdx := -1

	for idx, priceRecord := range records {
		if priceRecord.Timestamp < from {
			omitted = true
			continue
		}

		offsetIdx = idx
		break
	}

	if offsetIdx < 0 {
		return nil, omitted
	}

	return records[offsetIdx:], omitted
}

// GetStandardDeviationForPriceRecords returns the arithmetic mean for the price records.
func (k *Keeper) GetStandardDeviationForPriceRecords(priceRecords []*types.PriceRecord) *math.LegacyDec {
	if len(priceRecords) == 1 {
		standardDeviationValue := math.LegacyZeroDec()
		return &standardDeviationValue
	}

	sum := math.LegacyZeroDec()
	for _, priceRecord := range priceRecords {
		sum = sum.Add(priceRecord.Price)
	}

	// x̄ = ∑p / n, where n = number of priceRecords
	mean := k.GetMeanForPriceRecords(priceRecords)

	sum = math.LegacyZeroDec()
	for _, priceRecord := range priceRecords {
		deviation := priceRecord.Price.Sub(mean)
		sum = sum.Add(deviation.Mul(deviation))
	}

	// σ² = ∑((p - x̄)²) / n
	variance := sum.Quo(math.LegacyNewDec(int64(len(priceRecords))))
	// σ = √σ²
	standardDeviationValue, err := variance.ApproxSqrt()
	if err != nil {
		return nil
	}

	return &standardDeviationValue
}

// GetMeanForPriceRecords returns the arithmetic mean for the price records.
// x̄ = ∑p / n
func (k *Keeper) GetMeanForPriceRecords(priceRecords []*types.PriceRecord) (mean math.LegacyDec) {
	if len(priceRecords) == 0 {
		return math.LegacyZeroDec()
	}

	sum := math.LegacyZeroDec()
	for _, priceRecord := range priceRecords {
		sum = sum.Add(priceRecord.Price)
	}

	return sum.Quo(math.LegacyNewDec(int64(len(priceRecords))))
}

// CalculateStatistics returns statistics metadata over given price records
func CalculateStatistics(priceRecords []*types.PriceRecord) *types.MetadataStatistics {
	var (
		sum, twapSum = math.LegacyZeroDec(), math.LegacyZeroDec()
		count        = uint32(len(priceRecords))
	)
	if count == 0 {
		return nil
	}

	for i, r := range priceRecords {
		sum = sum.Add(r.Price)
		if i > 0 {
			// twapSum += p * ∆t
			twapSum = twapSum.Add(r.Price.Mul(math.LegacyNewDec(r.Timestamp - priceRecords[i-1].Timestamp)))
		}
	}

	// compute median on copy so the slice sorting doesn't mess up the indexes above
	recordsCopy := make([]*types.PriceRecord, 0, count)
	recordsCopy = append(recordsCopy, priceRecords...)
	slices.SortStableFunc(recordsCopy, func(a, b *types.PriceRecord) int {
		return a.Price.BigInt().Cmp(b.Price.BigInt())
	})

	median := recordsCopy[count/2].Price
	if count%2 == 0 {
		median = median.Add(recordsCopy[count/2-1].Price).Quo(math.LegacyNewDec(2))
	}

	meta := &types.MetadataStatistics{
		Mean:              sum.Quo(math.LegacyNewDec(int64(count))),
		MinPrice:          recordsCopy[0].Price,
		MaxPrice:          recordsCopy[count-1].Price,
		MedianPrice:       median,
		FirstTimestamp:    priceRecords[0].Timestamp,
		LastTimestamp:     priceRecords[count-1].Timestamp,
		GroupCount:        count,
		RecordsSampleSize: count,
		Twap:              math.LegacyZeroDec(),
	}
	if count > 1 {
		meta.Twap = twapSum.Quo(math.LegacyNewDec(meta.LastTimestamp - meta.FirstTimestamp))
	}

	return meta
}

func (k *Keeper) GetOracleVolatility(
	ctx sdk.Context,
	base, quote *types.OracleInfo,
	options *types.OracleHistoryOptions,
) (vol *math.LegacyDec, points []*types.PriceRecord, meta *types.MetadataStatistics) {
	defer k.Meter(ctx).FuncTiming(&ctx, "GetOracleVolatility")()

	var priceRecords *types.PriceRecords

	maxAge := ctx.BlockTime().Unix() - int64(types.MaxHistoricalPriceRecordAge)
	includeRawHistory := false
	includeMetadata := false

	if options != nil {
		if options.MaxAge > 0 {
			maxAge = ctx.BlockTime().Unix() - int64(options.MaxAge)
		}
		includeRawHistory = options.IncludeRawHistory
		includeMetadata = options.IncludeMetadata
	}

	if quote == nil || quote.Symbol == types.QuoteUSD {
		priceRecords = k.GetHistoricalPriceRecords(ctx, base.OracleType, base.Symbol, maxAge)
	} else {
		priceRecords, _ = k.GetMixedHistoricalPriceRecords(ctx, base.OracleType, quote.OracleType, base.Symbol, quote.Symbol, maxAge)
	}

	if priceRecords == nil || len(priceRecords.LatestPriceRecords) == 0 {
		return
	}

	vol = k.GetStandardDeviationForPriceRecords(priceRecords.LatestPriceRecords)

	if includeRawHistory {
		points = priceRecords.LatestPriceRecords
	}

	if includeMetadata {
		meta = CalculateStatistics(priceRecords.LatestPriceRecords)
	}

	return vol, points, meta
}
