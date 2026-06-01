package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func setup(t *testing.T) (*Pipeline, string, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	db, _ := agentdb.Open(filepath.Join(dir, "agent.db"))

	syncCalled := false
	paisaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync" {
			syncCalled = true
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(func() {
		paisaSrv.Close()
		_ = syncCalled
	})

	mockTx := parser.ParsedTransaction{
		Date: "2025-05-14", Amount: -2450, Currency: "INR",
		Merchant: "Swiggy", Bank: "HDFC", AccountLast4: "1234",
		RefID: "REF001", TxType: "debit",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
		Confidence:             0.95,
	}
	ollamaContent, _ := json.Marshal(mockTx)
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(ollamaContent)},
		})
	}))
	t.Cleanup(ollamaSrv.Close)

	cfg := agentconfig.DefaultConfig()
	cfg.Paisa.URL = paisaSrv.URL
	cfg.Paisa.JournalDir = dir
	cfg.Ollama.URL = ollamaSrv.URL
	cfg.Accounts = map[string]string{"HDFC:1234": "Assets:HDFC:Savings"}

	p := New(db, cfg)
	return p, dir, paisaSrv
}

func TestProcess_KnownMerchantAutoPost(t *testing.T) {
	p, dir, _ := setup(t)
	p.db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})

	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	assert.NoError(t, err)
	assert.Equal(t, ActionPosted, action)

	data, _ := os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
	assert.Contains(t, string(data), "Swiggy")
}

func TestProcess_UnknownMerchantNeedsApproval(t *testing.T) {
	p, _, _ := setup(t)
	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	assert.Equal(t, ActionPendingApproval, action)
	_ = err
}

func TestParseCallback(t *testing.T) {
	action, refID := parseCallback("approve:REF001")
	assert.Equal(t, "approve", action)
	assert.Equal(t, "REF001", refID)

	action, refID = parseCallback("skip:REF-XYZ-99")
	assert.Equal(t, "skip", action)
	assert.Equal(t, "REF-XYZ-99", refID)

	action, refID = parseCallback("badformat")
	assert.Equal(t, "", action)
	assert.Equal(t, "", refID)
}

func TestProcess_DuplicateSkipped(t *testing.T) {
	p, dir, _ := setup(t)
	p.db.Create(&agentdb.MerchantRule{Merchant: "Swiggy", Account: "Expenses:Food:Dining", ApproveCount: 5, AutoApprove: true})

	_, _ = p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "sms")
	action, err := p.Process("HDFC: Rs.2450 debited at SWIGGY Ref REF001", "gmail_alert")
	assert.NoError(t, err)
	assert.Equal(t, ActionDuplicate, action)

	data, _ := os.ReadFile(filepath.Join(dir, "auto-import.ledger"))
	assert.Equal(t, 1, strings.Count(string(data), "2025/05/14 Swiggy"))
}
