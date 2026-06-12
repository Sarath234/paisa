// internal/agent/llm/intent.go
package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

// Intent is one routable intent label with a description for the classifier prompt.
type Intent struct {
	Name        string
	Description string
}

// ClassifyIntent asks Ollama to label a message with one of the given intents.
// Returns "unknown" (no error) when the model picks a label outside the list.
func ClassifyIntent(text string, intents []Intent, cfg config.OllamaConfig) (string, error) {
	var lines []string
	for _, in := range intents {
		lines = append(lines, fmt.Sprintf("- %s: %s", in.Name, in.Description))
	}

	prompt := fmt.Sprintf(
		"Classify this message into exactly one intent.\n\nMessage: %s\n\nIntents:\n%s\n- unknown: none of the above\n\nReply with ONLY a JSON object like {\"intent\": \"...\"}. No explanation.",
		text, strings.Join(lines, "\n"),
	)

	response, err := Generate(prompt, cfg)
	if err != nil {
		return "", err
	}

	var result struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(ExtractJSON(response)), &result); err != nil {
		return "", fmt.Errorf("intent parse: %w", err)
	}

	name := strings.ToLower(strings.TrimSpace(result.Intent))
	for _, in := range intents {
		if in.Name == name {
			log.Infof("llm: intent classified as %q", name)
			return name, nil
		}
	}
	log.Infof("llm: unlisted intent %q — returning unknown", name)
	return "unknown", nil
}
