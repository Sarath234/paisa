// internal/agent/llm/intent_test.go
package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/config"
)

func intentServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response": "` + response + `"}`))
	}))
}

var testIntents = []Intent{
	{Name: "sms_ingest", Description: "a bank transaction SMS"},
	{Name: "finance_qa", Description: "a question about the user's finances"},
}

func TestClassifyIntent(t *testing.T) {
	srv := intentServer(t, `{\"intent\": \"finance_qa\"}`)
	defer srv.Close()
	got, err := ClassifyIntent("how much did I spend?", testIntents, config.OllamaConfig{URL: srv.URL})
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if got != "finance_qa" {
		t.Errorf("got %q, want finance_qa", got)
	}
}

func TestClassifyIntentUnknownLabel(t *testing.T) {
	srv := intentServer(t, `{\"intent\": \"weather_report\"}`)
	defer srv.Close()
	got, err := ClassifyIntent("hello", testIntents, config.OllamaConfig{URL: srv.URL})
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}

func TestClassifyIntentOllamaDown(t *testing.T) {
	_, err := ClassifyIntent("hello", testIntents, config.OllamaConfig{URL: "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("expected error")
	}
}
