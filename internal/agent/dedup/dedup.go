package dedup

import (
	"math"
	"time"

	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"gorm.io/gorm"
)

type Result int

const (
	New       Result = iota
	Duplicate        // exact ref_id match
	Fuzzy            // same account+amount within ±1 day
)

func Check(db *gorm.DB, refID, date, account string, amount float64) Result {
	if refID != "" {
		var count int64
		db.Model(&agentdb.ImportedRef{}).Where("ref_id = ?", refID).Count(&count)
		if count > 0 {
			return Duplicate
		}
	}

	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return New
	}

	var refs []agentdb.ImportedRef
	db.Where("account = ? AND date >= ? AND date <= ?",
		account,
		t.AddDate(0, 0, -1).Format("2006-01-02"),
		t.AddDate(0, 0, 1).Format("2006-01-02"),
	).Find(&refs)

	for _, r := range refs {
		if math.Abs(r.Amount-amount) < 0.01 {
			return Fuzzy
		}
	}

	return New
}

func Record(db *gorm.DB, refID, date, account, source string, amount float64) error {
	return db.Create(&agentdb.ImportedRef{
		RefID:   refID,
		Date:    date,
		Amount:  amount,
		Account: account,
		Source:  source,
	}).Error
}
