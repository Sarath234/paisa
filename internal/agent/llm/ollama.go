// internal/agent/llm/ollama.go
package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
	log "github.com/sirupsen/logrus"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

type fillResult struct {
	Desc string `json:"desc"`
	Dest string `json:"dest"`
}

// FillMissing calls Ollama to populate empty Desc and/or Dest fields in the entry.
// Only overwrites fields that are currently empty.
func FillMissing(sms string, entry *ledger.Entry, cfg config.OllamaConfig) error {
	var missing []string
	if entry.Desc == "" {
		missing = append(missing, "desc (short payee description, e.g. \"Food Swiggy\")")
	}
	if entry.Dest == "" {
		missing = append(missing, "dest (ledger account, e.g. \"Expenses:Food:Hyd\")")
	}
	if len(missing) == 0 {
		log.Debugf("llm: all fields present, skipping LLM call")
		return nil
	}

	log.Infof("llm: calling Ollama model=%q for missing fields: %v", cfg.Model, missing)

	prompt := fmt.Sprintf(
		"You are a personal finance assistant. Given this bank SMS, extract only the missing fields.\n\nSMS: %s\n\nAlready known: date=%s, src=%s, amt=%s\n\nMissing: %s\n\nReply with ONLY a JSON object like {\"desc\": \"...\", \"dest\": \"...\"} containing the missing fields. No explanation.",
		sms, entry.Date, entry.Src, entry.Amt, strings.Join(missing, ", "),
	)

	response, err := Generate(prompt, cfg)
	if err != nil {
		return err
	}

	jsonStr := extractJSON(response)
	var result fillResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Warnf("llm: could not extract JSON from response %q: %v", truncate(response, 200), err)
		return fmt.Errorf("ollama result parse: %w", err)
	}

	if entry.Desc == "" && result.Desc != "" {
		log.Infof("llm: filled desc=%q", result.Desc)
		entry.Desc = result.Desc
	}
	if entry.Dest == "" && result.Dest != "" {
		log.Infof("llm: filled dest=%q", result.Dest)
		entry.Dest = result.Dest
	}
	if entry.Desc == "" {
		log.Warnf("llm: desc still empty after LLM fill")
	}
	if entry.Dest == "" {
		log.Warnf("llm: dest still empty after LLM fill")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// extractJSON finds the first {...} JSON object in a string (handles markdown code blocks).
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "{}"
	}
	return s[start : end+1]
}
