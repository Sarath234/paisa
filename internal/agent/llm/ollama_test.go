// internal/agent/llm/ollama_test.go
package llm_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	"github.com/ananthakumaran/paisa/internal/agent/llm"
	"github.com/stretchr/testify/assert"
)

func TestFillMissing_FillsBothFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"response": `{"desc": "Grocery Shopping", "dest": "Expenses:Groceries:Hyd"}`,
		})
	}))
	defer srv.Close()

	entry := &ledger.Entry{Date: "2026/06/03", Src: "Assets:Checking:FC2148", Amt: "-500.00 INR"}
	cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
	err := llm.FillMissing("some unknown SMS", entry, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "Grocery Shopping", entry.Desc)
	assert.Equal(t, "Expenses:Groceries:Hyd", entry.Dest)
}

func TestFillMissing_PreservesExistingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"response": `{"desc": "LLM guess", "dest": "Expenses:Unknown"}`,
		})
	}))
	defer srv.Close()

	entry := &ledger.Entry{Desc: "Already set", Dest: "Already:Set"}
	cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
	err := llm.FillMissing("sms", entry, cfg)
	assert.NoError(t, err)
	// Existing non-empty fields must not be overwritten
	assert.Equal(t, "Already set", entry.Desc)
	assert.Equal(t, "Already:Set", entry.Dest)
}

func TestFillMissing_LLMMarkdownWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"response": "```json\n{\"desc\": \"Travel\", \"dest\": \"Expenses:Travel:Hyd\"}\n```",
		})
	}))
	defer srv.Close()

	entry := &ledger.Entry{}
	cfg := config.OllamaConfig{URL: srv.URL, Model: "test-model"}
	err := llm.FillMissing("sms", entry, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "Travel", entry.Desc)
	assert.Equal(t, "Expenses:Travel:Hyd", entry.Dest)
}
