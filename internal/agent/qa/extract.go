// internal/agent/qa/extract.go
package qa

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	log "github.com/sirupsen/logrus"
)

// Query is the structured form of a finance question. The LLM only fills
// these fields; all numbers are computed in Go.
type Query struct {
	Intent   string `json:"intent"`
	Category string `json:"category"`
	Account  string `json:"account"`
	Period   string `json:"period"`
}

var validIntents = map[string]bool{
	"expense_summary": true,
	"networth":        true,
	"account_balance": true,
	"budget_status":   true,
}

const extractPrompt = `You are a personal finance assistant. Extract the structured query from this question.

Question: %s

Intents:
- expense_summary: how much was spent (optionally on a category, in a period)
- networth: current net worth / total wealth
- account_balance: balance of a specific account
- budget_status: budget remaining or overspent (optionally for a category)

Reply with ONLY a JSON object like {"intent": "...", "category": "...", "account": "...", "period": "..."}.
Omit fields that don't apply. period examples: "this_month", "last_month", "this_year", "may", "may 2026". No explanation.`

// Extract turns a natural-language question into a Query via one Ollama call.
func Extract(text string, cfg config.OllamaConfig) (*Query, error) {
	response, err := llm.Generate(fmt.Sprintf(extractPrompt, text), cfg)
	if err != nil {
		return nil, err
	}

	var q Query
	if err := json.Unmarshal([]byte(llm.ExtractJSON(response)), &q); err != nil {
		return nil, fmt.Errorf("extract parse: %w", err)
	}
	q.Intent = strings.ToLower(strings.TrimSpace(q.Intent))
	if !validIntents[q.Intent] {
		return nil, fmt.Errorf("unrecognized intent %q", q.Intent)
	}
	log.Infof("qa: extracted intent=%q category=%q account=%q period=%q", q.Intent, q.Category, q.Account, q.Period)
	return &q, nil
}
