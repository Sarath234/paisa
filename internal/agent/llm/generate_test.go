// internal/agent/llm/generate_test.go
package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"response": "hello {\"a\": 1}"}`))
	}))
	defer srv.Close()

	got, err := Generate("prompt", config.OllamaConfig{URL: srv.URL, Model: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != `hello {"a": 1}` {
		t.Errorf("got %q", got)
	}
}

func TestGenerateServerDown(t *testing.T) {
	_, err := Generate("prompt", config.OllamaConfig{URL: "http://127.0.0.1:1", Model: "test"})
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestExtractJSONExported(t *testing.T) {
	if got := ExtractJSON("```json\n{\"x\":1}\n```"); got != `{"x":1}` {
		t.Errorf("got %q", got)
	}
	if got := ExtractJSON("no json here"); got != "{}" {
		t.Errorf("got %q", got)
	}
}
