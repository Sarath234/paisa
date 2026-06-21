package service

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/price"
	"github.com/ananthakumaran/paisa/internal/utils"
	"github.com/google/btree"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type priceCache struct {
	// postingPricesTree holds posting-derived (unknown-type) prices.
	// Immutable after buildPriceCache; no lock needed.
	postingPricesTree map[string]*btree.BTree

	// latestKnownPrice holds the single most-recent external price per commodity.
	// Built eagerly via a cheap GROUP BY query; used by the "today" fast path.
	// Immutable after buildPriceCache; no lock needed.
	latestKnownPrice map[string]price.Price

	// pricesTree holds full external price history, lazily loaded per commodity
	// on first historical lookup. Protected by mu.
	mu          sync.RWMutex
	pricesTree  map[string]*btree.BTree
	knownLoaded map[string]bool
}

var (
	pcachePtr  atomic.Pointer[priceCache]
	pcacheLoad sync.Mutex
)

func buildPriceCache(db *gorm.DB) *priceCache {
	c := &priceCache{
		postingPricesTree: make(map[string]*btree.BTree),
		latestKnownPrice:  make(map[string]price.Price),
		pricesTree:        make(map[string]*btree.BTree),
		knownLoaded:       make(map[string]bool),
	}

	// Load distinct non-currency commodities from postings.
	var allCommodities []string
	if r := db.Model(&posting.Posting{}).Distinct().Pluck("commodity", &allCommodities); r.Error != nil {
		log.Fatal(r.Error)
	}

	// Eagerly load posting-derived (unknown-type) prices — fast fallback.
	var unknownPrices []price.Price
	if r := db.Where("commodity_type = ?", config.Unknown).Find(&unknownPrices); r.Error != nil {
		log.Fatal(r.Error)
	}
	unknownByName := lo.GroupBy(unknownPrices, func(p price.Price) string { return p.CommodityName })
	for _, name := range allCommodities {
		if !utils.IsCurrency(name) {
			tree := btree.New(2)
			for _, p := range unknownByName[name] {
				tree.ReplaceOrInsert(p)
			}
			c.postingPricesTree[name] = tree
		}
	}

	// Eagerly load the single most-recent external price per commodity.
	// One cheap query (~90 rows) replaces the original full-history bulk load.
	var latestPrices []price.Price
	if r := db.Raw(`
		SELECT p.* FROM prices p
		INNER JOIN (
			SELECT commodity_name, MAX(date) AS max_date
			FROM prices
			WHERE commodity_type != ?
			GROUP BY commodity_name
		) AS latest ON p.commodity_name = latest.commodity_name AND p.date = latest.max_date
		WHERE p.commodity_type != ?`, config.Unknown, config.Unknown).Scan(&latestPrices); r.Error != nil {
		log.Warn("load latest known prices: ", r.Error)
	}
	for _, p := range latestPrices {
		c.latestKnownPrice[p.CommodityName] = p
	}

	// Full external price histories are loaded lazily per commodity on first
	// historical lookup (see loadKnownPrices / GetUnitPrice).
	return c
}

// loadKnownPrices ensures the full external price history for commodity is in
// c.pricesTree. Safe for concurrent callers; loads from DB at most once per
// commodity per cache lifetime.
func loadKnownPrices(db *gorm.DB, c *priceCache, commodity string) {
	c.mu.RLock()
	loaded := c.knownLoaded[commodity]
	c.mu.RUnlock()
	if loaded {
		return
	}

	// Do the DB query outside any lock so concurrent requests don't serialise.
	var prices []price.Price
	if r := db.Where("commodity_type != ? AND commodity_name = ?", config.Unknown, commodity).Find(&prices); r.Error != nil {
		log.Warnf("load known prices for %s: %v", commodity, r.Error)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.knownLoaded[commodity] {
		return
	}
	if len(prices) > 0 {
		tree := btree.New(2)
		for _, p := range prices {
			tree.ReplaceOrInsert(p)
		}
		c.pricesTree[commodity] = tree
	}
	c.knownLoaded[commodity] = true
}

func ensureCache(db *gorm.DB) *priceCache {
	if c := pcachePtr.Load(); c != nil {
		return c
	}
	pcacheLoad.Lock()
	defer pcacheLoad.Unlock()
	if c := pcachePtr.Load(); c != nil {
		return c
	}
	c := buildPriceCache(db)
	pcachePtr.Store(c)
	return c
}

func ClearPriceCache() {
	pcachePtr.Store(nil)
}

func GetUnitPrice(db *gorm.DB, commodity string, date time.Time) price.Price {
	c := ensureCache(db)

	if utils.IsCurrency(commodity) {
		return price.Price{}
	}

	today := utils.EndOfToday()

	// Fast path: for today-or-later queries use the pre-loaded latest price.
	// PopulateMarketPrice always uses EndOfToday(), so this covers the hot path
	// without ever loading full price history.
	if !date.Before(today) {
		if p, ok := c.latestKnownPrice[commodity]; ok {
			return p
		}
		// Fall through to posting-derived prices.
		if pt := c.postingPricesTree[commodity]; pt != nil {
			return utils.BTreeDescendFirstLessOrEqual(pt, price.Price{Date: date})
		}
		log.Warn("No price found for commodity: ", commodity)
		return price.Price{}
	}

	// Historical path: lazy-load full price history for this commodity.
	loadKnownPrices(db, c, commodity)

	c.mu.RLock()
	pt := c.pricesTree[commodity]
	c.mu.RUnlock()

	if pt != nil {
		pc := utils.BTreeDescendFirstLessOrEqual(pt, price.Price{Date: date})
		if !pc.Value.Equal(decimal.Zero) {
			return pc
		}
	}

	if pt := c.postingPricesTree[commodity]; pt != nil {
		return utils.BTreeDescendFirstLessOrEqual(pt, price.Price{Date: date})
	}
	log.Warn("No price tree found for commodity: ", commodity)
	return price.Price{}
}

func GetAllPrices(db *gorm.DB, commodity string) []price.Price {
	c := ensureCache(db)

	if !utils.IsCurrency(commodity) {
		loadKnownPrices(db, c, commodity)
	}

	pmap := make(map[string]price.Price)

	if pt := c.postingPricesTree[commodity]; pt != nil {
		for _, p := range utils.BTreeToSlice[price.Price](pt) {
			pmap[p.Date.String()] = p
		}
	}

	c.mu.RLock()
	pt := c.pricesTree[commodity]
	c.mu.RUnlock()

	if pt != nil {
		for _, p := range utils.BTreeToSlice[price.Price](pt) {
			pmap[p.Date.String()] = p
		}
	}

	if len(pmap) == 0 {
		log.Warn("No prices found for commodity: ", commodity)
		return []price.Price{}
	}

	prices := make([]price.Price, 0, len(pmap))
	keys := lo.Keys(pmap)
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	for _, key := range keys {
		prices = append(prices, pmap[key])
	}

	return prices
}

func GetMarketPrice(db *gorm.DB, p posting.Posting, date time.Time) decimal.Decimal {
	if utils.IsCurrency(p.Commodity) {
		return p.Amount
	}

	pc := GetUnitPrice(db, p.Commodity, date)
	if !pc.Value.Equal(decimal.Zero) {
		return p.Quantity.Mul(pc.Value)
	}

	return p.Amount
}

func GetPrice(db *gorm.DB, commodity string, quantity decimal.Decimal, date time.Time) decimal.Decimal {
	if utils.IsCurrency(commodity) {
		return quantity
	}

	pc := GetUnitPrice(db, commodity, date)
	if !pc.Value.Equal(decimal.Zero) {
		return quantity.Mul(pc.Value)
	}

	return quantity
}

func PopulateMarketPrice(db *gorm.DB, ps []posting.Posting) []posting.Posting {
	date := utils.EndOfToday()
	return lo.Map(ps, func(p posting.Posting, _ int) posting.Posting {
		p.MarketAmount = GetMarketPrice(db, p, date)
		return p
	})
}
