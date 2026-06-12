// internal/agent/qa/extract_test.go
package qa

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

func ollamaStub(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response": "` + response + `"}`))
	}))
}

func TestExtract(t *testing.T) {
	srv := ollamaStub(t, `{\"intent\": \"expense_summary\", \"category\": \"food\", \"period\": \"this_month\"}`)
	defer srv.Close()
	q, err := Extract("how much did I spend on food this month?", config.OllamaConfig{URL: srv.URL})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if q.Intent != "expense_summary" || q.Category != "food" || q.Period != "this_month" {
		t.Errorf("got %+v", q)
	}
}

func TestExtractInvalidIntent(t *testing.T) {
	srv := ollamaStub(t, `{\"intent\": \"stock_tips\"}`)
	defer srv.Close()
	if _, err := Extract("give me stock tips", config.OllamaConfig{URL: srv.URL}); err == nil {
		t.Fatal("expected error for out-of-enum intent")
	}
}

func TestExtractMalformedJSON(t *testing.T) {
	srv := ollamaStub(t, `sorry I cannot help`)
	defer srv.Close()
	if _, err := Extract("hello", config.OllamaConfig{URL: srv.URL}); err == nil {
		t.Fatal("expected error for non-JSON response")
	}
}
