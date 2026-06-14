package model

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/ananthakumaran/paisa/internal/ledger"
	"github.com/ananthakumaran/paisa/internal/model/cache"
	"github.com/ananthakumaran/paisa/internal/model/cii"
	"github.com/ananthakumaran/paisa/internal/model/commodity"
	mutualfundModel "github.com/ananthakumaran/paisa/internal/model/mutualfund/scheme"
	npsModel "github.com/ananthakumaran/paisa/internal/model/nps/scheme"
	"github.com/ananthakumaran/paisa/internal/model/portfolio"
	"github.com/ananthakumaran/paisa/internal/model/posting"
	"github.com/ananthakumaran/paisa/internal/model/price"
	"github.com/ananthakumaran/paisa/internal/scraper"
	"github.com/ananthakumaran/paisa/internal/scraper/india"
	"github.com/ananthakumaran/paisa/internal/scraper/mutualfund"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) {
	db.AutoMigrate(&npsModel.Scheme{})
	db.AutoMigrate(&mutualfundModel.Scheme{})
	db.AutoMigrate(&posting.Posting{})
	db.AutoMigrate(&price.Price{})
	db.AutoMigrate(&portfolio.Portfolio{})
	db.AutoMigrate(&price.Price{})
	db.AutoMigrate(&cii.CII{})
	db.AutoMigrate(&cache.Cache{})
}

func SyncJournal(db *gorm.DB) (string, error) {
	AutoMigrate(db)
	log.Info("Syncing transactions from journal")

	journalPath := config.GetJournalPath()

	type pricesResult struct {
		prices []price.Price
		err    error
	}
	type postingsResult struct {
		postings []*posting.Posting
		err      error
	}

	pricesCh := make(chan pricesResult, 1)
	postingsCh := make(chan postingsResult, 1)

	go func() {
		ps, err := ledger.Cli().Prices(journalPath)
		pricesCh <- pricesResult{ps, err}
	}()

	go func() {
		ps, err := ledger.Cli().Parse(journalPath, nil)
		postingsCh <- postingsResult{ps, err}
	}()

	pr := <-pricesCh
	if pr.err != nil {
		return pr.err.Error(), pr.err
	}

	por := <-postingsCh
	if por.err != nil {
		return por.err.Error(), por.err
	}

	price.UpsertAllByType(db, config.Unknown, pr.prices)
	posting.UpsertAll(db, por.postings)

	return "", nil
}

func SyncCommodities(db *gorm.DB) error {
	AutoMigrate(db)
	log.Info("Fetching commodities price history")
	commodities := lo.Shuffle(commodity.All())

	type fetchResult struct {
		commodity config.Commodity
		prices    []*price.Price
		err       error
	}

	// Fetch prices concurrently (network-bound); collect results for sequential DB writes.
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	results := make(chan fetchResult, len(commodities))
	var wg sync.WaitGroup

	for _, c := range commodities {
		c := c
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					results <- fetchResult{commodity: c, err: fmt.Errorf("panic fetching price for %s: %v", c.Name, r)}
				}
			}()

			log.Info("Fetching commodity ", c.Name)
			prices, err := scraper.GetProviderByCode(c.Price.Provider).GetPrices(c.Price.Code, c.Name)
			results <- fetchResult{commodity: c, prices: prices, err: err}
		}()
	}
	wg.Wait()
	close(results)

	// Write to DB sequentially to avoid SQLite write contention.
	var errors []error
	for r := range results {
		if r.err != nil {
			log.Error(r.err)
			errors = append(errors, fmt.Errorf("Failed to fetch price for %s: %w", r.commodity.Name, r.err))
			continue
		}
		price.UpsertAllByTypeNameAndID(db, r.commodity.Type, r.commodity.Name, r.commodity.Price.Code, r.prices)
	}

	if len(errors) > 0 {
		var message string
		for _, error := range errors {
			message += error.Error() + "\n"
		}
		return fmt.Errorf("%s", strings.Trim(message, "\n"))
	}
	return nil
}

func SyncCII(db *gorm.DB) error {
	AutoMigrate(db)
	log.Info("Fetching taxation related info")
	ciis, err := india.GetCostInflationIndex()
	if err != nil {
		log.Error(err)
		return fmt.Errorf("Failed to fetch CII: %w", err)
	}
	cii.UpsertAll(db, ciis)
	return nil
}

func SyncPortfolios(db *gorm.DB) error {
	db.AutoMigrate(&portfolio.Portfolio{})
	log.Info("Fetching commodities portfolio")
	commodities := commodity.FindByType(config.MutualFund)
	for _, commodity := range commodities {
		if commodity.Price.Provider != "in-mfapi" {
			continue
		}

		name := commodity.Name
		log.Info("Fetching portfolio for ", name)
		portfolios, err := mutualfund.GetPortfolio(commodity.Price.Code, commodity.Name)

		if err != nil {
			log.Error(err)
			return fmt.Errorf("Failed to fetch portfolio for %s: %w", name, err)
		}

		portfolio.UpsertAll(db, commodity.Type, commodity.Price.Code, portfolios)
	}
	return nil
}
