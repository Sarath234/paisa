package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/stretchr/testify/assert"
)

func TestGetUpdates_ParsesMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"update_id": 1,
					"message": map[string]interface{}{
						"message_id": 101,
						"text":       "hello",
						"chat":       map[string]interface{}{"id": 42},
					},
				},
				{
					"update_id": 2,
					"callback_query": map[string]interface{}{
						"id":   "cb1",
						"data": "approve:ref123",
						"message": map[string]interface{}{
							"message_id": 102,
							"text":       "card",
							"chat":       map[string]interface{}{"id": 42},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	bot := newWithBaseURL("token", 42, srv.URL)
	updates, err := bot.GetUpdates()
	assert.NoError(t, err)
	assert.Len(t, updates, 2)
	assert.NotNil(t, updates[0].Message)
	assert.Equal(t, "hello", updates[0].Message.Text)
	assert.NotNil(t, updates[1].CallbackQuery)
	assert.Equal(t, "approve:ref123", updates[1].CallbackQuery.Data)
}

func TestGetUpdates_AdvancesOffset(t *testing.T) {
	callCount := 0
	var lastOffset string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		lastOffset = r.URL.Query().Get("offset")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": []map[string]interface{}{
				{"update_id": 2},
			},
		})
	}))
	defer srv.Close()

	bot := newWithBaseURL("token", 42, srv.URL)
	_, _ = bot.GetUpdates()
	assert.Equal(t, "0", lastOffset)

	_, _ = bot.GetUpdates()
	assert.Equal(t, "3", lastOffset)
	assert.Equal(t, 2, callCount)
}

func TestSendApprovalCard_SendsMessage(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	bot := newWithBaseURL("token", 42, srv.URL)
	tx := parser.ParsedTransaction{
		Date:                   "2025-05-14",
		Amount:                 -2450.00,
		Currency:               "INR",
		Merchant:               "Swiggy",
		Bank:                   "HDFC",
		AccountLast4:           "1234",
		RefID:                  "ref999",
		SuggestedLedgerAccount: "Expenses:Food:Dining",
	}

	err := bot.SendApprovalCard(tx)
	assert.NoError(t, err)
	assert.Contains(t, body["text"], "Swiggy")
	assert.NotNil(t, body["reply_markup"])
}
