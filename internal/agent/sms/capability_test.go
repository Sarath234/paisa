// internal/agent/sms/capability_test.go
package sms

import (
	"strings"
	"testing"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/ledger"
)

type fakeBot struct {
	texts  []string
	drafts []string
}

func (f *fakeBot) SendText(text string) error { f.texts = append(f.texts, text); return nil }
func (f *fakeBot) SendDraft(text string) (int, error) {
	f.drafts = append(f.drafts, text)
	return len(f.drafts), nil
}
func (f *fakeBot) SendDraftDuplicate(text string) (int, error) {
	f.drafts = append(f.drafts, text)
	return len(f.drafts), nil
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Paisa:    config.PaisaConfig{JournalDir: t.TempDir()},
		Telegram: config.TelegramConfig{ChatID: 42}, // must match the chat ID used in tests: HandleReply looks up pending state by chat ID
		ParserRules: config.ParserRules{
			Accounts: []config.AccountRule{
				{Bank: "hdfc", Identifiers: []string{"HDFC Bank", "5587"}, Destinations: "Assets:Checking:HDFC"},
			},
		},
	}
}

func TestMatchKnownSMS(t *testing.T) {
	c := &Capability{Cfg: testConfig(t), Store: approval.NewStore()}
	if !c.Match("HDFC Bank: Rs 215.50 debited from a/c **5587 on 03-06-26 to VPA swiggy@ybl") {
		t.Error("SMS matching a configured account rule must fast-path match")
	}
}

func TestMatchRejectsQuestions(t *testing.T) {
	c := &Capability{Cfg: testConfig(t), Store: approval.NewStore()}
	if c.Match("how much did I spend on food this month?") {
		t.Error("natural language must not match")
	}
}

func TestName(t *testing.T) {
	if (&Capability{}).Name() != "sms_ingest" {
		t.Error("Name must be sms_ingest (router intent label)")
	}
}

func TestHasPendingFollowsEditingState(t *testing.T) {
	store := approval.NewStore()
	c := &Capability{Cfg: testConfig(t), Store: store}
	if c.HasPending(42) {
		t.Error("no pending state initially")
	}
	store.Set(&approval.Pending{
		Entry:     ledger.Entry{Desc: "test"},
		ChatID:    42,
		MessageID: 7,
		Status:    approval.StatusPending,
	})
	store.SetEditing(7)
	if !c.HasPending(42) {
		t.Error("editing state must surface as pending")
	}
}

func TestHandleReplyMergesEditAndResendsDraft(t *testing.T) {
	store := approval.NewStore()
	bot := &fakeBot{}
	c := &Capability{Cfg: testConfig(t), Store: store, Bot: bot}
	store.Set(&approval.Pending{
		Entry:     ledger.Entry{Desc: "Old", Date: "2026/06/03", Src: "Assets:Checking:HDFC", Amt: "-215.50 INR", Dest: "Expenses:Food"},
		ChatID:    42,
		MessageID: 7,
		Status:    approval.StatusPending,
	})
	store.SetEditing(7)

	if err := c.HandleReply(42, "desc: New Desc"); err != nil {
		t.Fatalf("HandleReply: %v", err)
	}
	if len(bot.drafts) != 1 || !strings.Contains(bot.drafts[0], "New Desc") {
		t.Errorf("edited draft must be re-sent: %v", bot.drafts)
	}
	if store.Get(7) != nil {
		t.Error("old pending entry must be deleted after edit")
	}
}

func TestHandleUnparseableSMSSendsError(t *testing.T) {
	bot := &fakeBot{}
	c := &Capability{Cfg: testConfig(t), Store: approval.NewStore(), Bot: bot}
	// matches no account rule → parse error path
	if err := c.Handle("random text that is not an SMS"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.texts) != 1 || !strings.Contains(bot.texts[0], "Could not parse") {
		t.Errorf("want parse-error message, got %v", bot.texts)
	}
}
