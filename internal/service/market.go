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
	pricesTree        map[string]*btree.BTree
	postingPricesTree map[string]*btree.BTree
}

// pcachePtr holds a *priceCache atomically. nil means uninitialised.
// ClearPriceCache stores nil; ensureCache does a compare-and-swap to populate it.
var (
	pcachePtr  atomic.Pointer[priceCache]
	pcacheLoad sync.Mutex // serialises concurrent initial loads
)

func buildPriceCache(db *gorm.DB) *priceCache {
	c := &priceCache{
		pricesTree:        make(map[string]*btree.BTree),
		postingPricesTree: make(map[string]*btree.BTree),
	}

	var prices []price.Price
	result := db.Where("commodity_type != ?", config.Unknown).Find(&prices)
	if result.Error != nil {
		log.Fatal(result.Error)
	}

	for _, p := range prices {
		if c.pricesTree[p.CommodityName] == nil {
			c.pricesTree[p.CommodityName] = btree.New(2)
		}
		c.pricesTree[p.CommodityName].ReplaceOrInsert(p)
	}

	// Load only distinct non-currency commodity names instead of all posting rows.
	var allCommodities []string
	if r := db.Model(&posting.Posting{}).Distinct().Pluck("commodity", &allCommodities); r.Error != nil {
		log.Fatal(r.Error)
	}

	// Bulk-load all Unknown-type prices in one query instead of N per-commodity queries.
	var unknownPrices []price.Price
	if r := db.Where("commodity_type = ?", config.Unknown).Find(&unknownPrices); r.Error != nil {
		log.Fatal(r.Error)
	}
	unknownByName := lo.GroupBy(unknownPrices, func(p price.Price) string { return p.CommodityName })

	for _, commodityName := range allCommodities {
		if !utils.IsCurrency(commodityName) {
			postingPricesTree := btree.New(2)
			for _, p := range unknownByName[commodityName] {
				postingPricesTree.ReplaceOrInsert(p)
			}
			c.postingPricesTree[commodityName] = postingPricesTree

			if c.pricesTree[commodityName] == nil {
				c.pricesTree[commodityName] = postingPricesTree
			}
		}
	}

	return c
}

// ensureCache returns the current priceCache, building it if needed.
// Safe for concurrent callers; only one goroutine will build the cache.
func ensureCache(db *gorm.DB) *priceCache {
	if c := pcachePtr.Load(); c != nil {
		return c
	}
	pcacheLoad.Lock()
	defer pcacheLoad.Unlock()
	// Double-check after acquiring the mutex.
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

func PreloadPriceCache(db *gorm.DB) {
	ensureCache(db)
}

func GetUnitPrice(db *gorm.DB, commodity string, date time.Time) price.Price {
	c := ensureCache(db)

	pt := c.pricesTree[commodity]
	if pt == nil {
		log.Warn("No price tree found for commodity: ", commodity)
		return price.Price{}
	}

	pc := utils.BTreeDescendFirstLessOrEqual(pt, price.Price{Date: date})
	if !pc.Value.Equal(decimal.Zero) {
		return pc
	}

	pt = c.postingPricesTree[commodity]
	if pt == nil {
		return price.Price{}
	}
	return utils.BTreeDescendFirstLessOrEqual(pt, price.Price{Date: date})
}

func GetAllPrices(db *gorm.DB, commodity string) []price.Price {
	c := ensureCache(db)

	pmap := make(map[string]price.Price)

	if pt := c.postingPricesTree[commodity]; pt != nil {
		for _, p := range utils.BTreeToSlice[price.Price](pt) {
			pmap[p.Date.String()] = p
		}
	}

	if pt := c.pricesTree[commodity]; pt != nil {
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
