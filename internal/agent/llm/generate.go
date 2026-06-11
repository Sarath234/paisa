// internal/agent/llm/generate.go
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

// Generate performs one non-streaming Ollama /api/generate call and returns
// the raw response text.
func Generate(prompt string, cfg config.OllamaConfig) (string, error) {
	body, _ := json.Marshal(generateRequest{Model: cfg.Model, Prompt: prompt, Stream: false})
	resp, err := httpClient.Post(cfg.URL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Warnf("llm: Ollama request failed url=%q: %v", cfg.URL, err)
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warnf("llm: could not read Ollama response body: %v", err)
		return "", fmt.Errorf("ollama response read: %w", err)
	}

	var gr generateResponse
	if err := json.Unmarshal(data, &gr); err != nil {
		log.Warnf("llm: could not parse Ollama envelope: %v (raw=%q)", err, truncate(string(data), 200))
		return "", fmt.Errorf("ollama response parse: %w", err)
	}
	log.Debugf("llm: raw response: %q", truncate(gr.Response, 300))
	return gr.Response, nil
}

// ExtractJSON finds the first {...} JSON object in a string (handles markdown
// code blocks). Returns "{}" when none found.
func ExtractJSON(s string) string {
	return extractJSON(s)
}
