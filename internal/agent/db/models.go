package db

import "gorm.io/gorm"

// ImportedRef tracks every transaction the agent has processed.
// Used for deduplication across sources.
type ImportedRef struct {
	gorm.Model
	RefID   string  `gorm:"index"`
	Date    string
	Amount  float64
	Account string
	Source  string // sms | gmail_alert | gmail_statement | skipped
}

// MerchantRule maps a merchant name to a ledger account and tracks
// how many times the user has manually approved it.
type MerchantRule struct {
	Merchant     string `gorm:"primaryKey"`
	Account      string
	ApproveCount int
	AutoApprove  bool
}
