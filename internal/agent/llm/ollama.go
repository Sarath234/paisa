// internal/agent/llm/ollama.go
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
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
		return nil
	}

	prompt := fmt.Sprintf(
		"You are a personal finance assistant. Given this bank SMS, extract only the missing fields.\n\nSMS: %s\n\nAlready known: date=%s, src=%s, amt=%s\n\nMissing: %s\n\nReply with ONLY a JSON object like {\"desc\": \"...\", \"dest\": \"...\"} containing the missing fields. No explanation.",
		sms, entry.Date, entry.Src, entry.Amt, strings.Join(missing, ", "),
	)

	body, _ := json.Marshal(generateRequest{Model: cfg.Model, Prompt: prompt, Stream: false})
	resp, err := httpClient.Post(cfg.URL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var gr generateResponse
	if err := json.Unmarshal(data, &gr); err != nil {
		return fmt.Errorf("ollama response parse: %w", err)
	}

	jsonStr := extractJSON(gr.Response)
	var result fillResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return fmt.Errorf("ollama result parse: %w", err)
	}

	if entry.Desc == "" && result.Desc != "" {
		entry.Desc = result.Desc
	}
	if entry.Dest == "" && result.Dest != "" {
		entry.Dest = result.Dest
	}
	return nil
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
