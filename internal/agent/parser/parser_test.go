package parser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse_ExtractsTransaction(t *testing.T) {
	mockResp := ParsedTransaction{
		Date:                   "2025-05-14",
		Amount:                 -2450.00,
		Currency:               "INR",
		Merchant:               "Swiggy",
		AccountLast4:           "1234",
		Bank:                   "HDFC",
		RefID:                  "47291830",
		TxType:                 "debit",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
		Confidence:             0.95,
	}
	content, _ := json.Marshal(mockResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(content)},
		})
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	tx, err := p.Parse("HDFC Bank: Rs.2,450.00 debited from a/c XX1234 at SWIGGY Ref 47291830", []string{"Expenses:Food:Dining"})

	assert.NoError(t, err)
	assert.Equal(t, "Swiggy", tx.Merchant)
	assert.Equal(t, -2450.00, tx.Amount)
	assert.Equal(t, "Expenses:Food:Dining", tx.SuggestedLedgerAccount)
	assert.Equal(t, 0.95, tx.Confidence)
}

func TestParse_HandlesOllamaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	_, err := p.Parse("some text", nil)
	assert.Error(t, err)
}

func TestParseMultiple_ExtractsManyTransactions(t *testing.T) {
	mockTxns := []ParsedTransaction{
		{Date: "2025-05-01", Amount: -500, Currency: "INR", Merchant: "Swiggy", RefID: "R1", TxType: "debit", Confidence: 0.9},
		{Date: "2025-05-03", Amount: -1200, Currency: "INR", Merchant: "Amazon", RefID: "R2", TxType: "debit", Confidence: 0.95},
	}
	content, _ := json.Marshal(mockTxns)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{"content": string(content)},
		})
	}))
	defer srv.Close()

	p := New(srv.URL, "gemma3:12b")
	txns, err := p.ParseMultiple("... PDF statement text ...", nil)
	assert.NoError(t, err)
	assert.Len(t, txns, 2)
	assert.Equal(t, "Swiggy", txns[0].Merchant)
	assert.Equal(t, "Amazon", txns[1].Merchant)
}
