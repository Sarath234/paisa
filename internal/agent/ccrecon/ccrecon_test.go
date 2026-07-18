// internal/agent/ccrecon/ccrecon_test.go
package ccrecon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/approval"
	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
	"github.com/ananthakumaran/paisa/internal/agent/statement"
)

const cardAcct = "Liabilities:CreditCard:ICIC6009"

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func txn(date, desc string, debit, credit float64, fee bool) statement.CCTransaction {
	return statement.CCTransaction{
		Transaction:     statement.Transaction{Date: day(date), Description: desc, Debit: debit, Credit: credit},
		IsInterestOrFee: fee,
	}
}

type fakeBot struct {
	texts, drafts, confirms []string
	edits                   map[int]string
	nextID                  int
}

func (f *fakeBot) SendText(s string) error { f.texts = append(f.texts, s); return nil }
func (f *fakeBot) SendDraft(s string) (int, error) {
	f.drafts = append(f.drafts, s)
	f.nextID++
	return f.nextID, nil
}
func (f *fakeBot) SendConfirm(s string) (int, error) {
	f.confirms = append(f.confirms, s)
	f.nextID++
	return f.nextID, nil
}
func (f *fakeBot) EditMessage(messageID int, text string) error {
	if f.edits == nil {
		f.edits = make(map[int]string)
	}
	f.edits[messageID] = text
	return nil
}

type fakeClient struct {
	postings  []paisaclient.Posting
	syncCalls int
}

func (f *fakeClient) Postings() ([]paisaclient.Posting, error) { return f.postings, nil }
func (f *fakeClient) SyncJournal() error                       { f.syncCalls++; return nil }

// stubParser is a fixture CCParser. name lets TestParserForRoutesByAccountInfix
// distinguish the map entries by Name(); it defaults to "icici_cc" so the
// single-field usage in the flow tests below keeps working.
type stubParser struct {
	res  statement.CCResult
	name string
}

func (s *stubParser) Name() string {
	if s.name == "" {
		return "icici_cc"
	}
	return s.name
}

func (s *stubParser) Parse(b []byte) (statement.CCResult, error) { return s.res, nil }

func TestHandleCCStatementFullFlow(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 23450.50, MinDue: 1180.00,
		Transactions: []statement.CCTransaction{
			txn("2026-06-15", "SWIGGY BANGALORE", 450.50, 0, false),
			txn("2026-06-20", "UNKNOWN MERCHANT", 999.00, 0, false), // missing in ledger
			txn("2026-07-05", "INTEREST CHARGES", 740.00, 0, true),  // missing + interest
		},
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		{Date: day("2026-06-16"), Account: cardAcct, Amount: -450.50, Payee: "Food Swiggy"},
	}}
	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: t.TempDir(), MaxCards: 10}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	// bill facts stored at pdf authority, incl. interest total
	b := store.BillsFor(cardAcct)[0]
	if b.TotalDue != 23450.50 || b.Sources["total_due"] != billtruth.AuthorityPDF {
		t.Fatalf("%+v", b)
	}
	if b.InterestTotal != 740.00 {
		t.Fatalf("interest total: %+v", b)
	}
	// summary sent, then 2 approval drafts (unknown merchant + interest)
	if len(bot.texts) < 1 || !strings.Contains(bot.texts[0], "1 matched") || !strings.Contains(bot.texts[0], "2 missing") {
		t.Fatalf("summary: %q", bot.texts)
	}
	if len(bot.drafts) != 2 {
		t.Fatalf("drafts: %q", bot.drafts)
	}
}

func TestHandleCCStatementCapsCards(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 50000,
	}
	for i := 0; i < 15; i++ {
		res.Transactions = append(res.Transactions,
			txn("2026-06-15", fmt.Sprintf("MERCHANT %d", i), float64(100+i), 0, false))
	}
	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: &fakeClient{}, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: t.TempDir(), MaxCards: 10}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.drafts) != 10 {
		t.Fatalf("cap: %d drafts", len(bot.drafts))
	}
	if !strings.Contains(bot.texts[0], "5 more") {
		t.Fatalf("summary must mention remainder: %q", bot.texts[0])
	}
}

func TestParserForRoutesByAccountInfix(t *testing.T) {
	parsers := map[string]statement.CCParser{
		"icici_cc": &stubParser{name: "icici_cc"},
		"axis_cc":  &stubParser{name: "axis_cc"},
		"hdfc_cc":  &stubParser{name: "hdfc_cc"},
	}
	cases := map[string]string{
		"Liabilities:CreditCard:ICIC6009":   "icici_cc",
		"Liabilities:CreditCard:SELECT6792": "axis_cc",
		"Liabilities:CreditCard:FK8860":     "axis_cc",
		"Liabilities:CreditCard:MyZone1610": "axis_cc",
		"Liabilities:CreditCard:HDFC2527":   "hdfc_cc",
	}
	for account, want := range cases {
		p, err := parserFor(account, parsers)
		if err != nil || p.Name() != want {
			t.Errorf("%s → %v, %v (want %s)", account, p, err, want)
		}
	}
	if _, err := parserFor("Liabilities:CreditCard:UNKNOWN1", parsers); err == nil {
		t.Error("unknown bank must error")
	}
}

func TestHandleCCStatementExtraSendsConfirmCard(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 1000,
		// no statement transactions -> the ledger posting below is unmatched ("extra")
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		{Date: day("2026-06-22"), Account: cardAcct, Amount: -999.00, Payee: "Dup Spend"},
	}}
	journalDir := t.TempDir()
	journal := "2026/06/22 Dup Spend\n    " + cardAcct + "               -999.00 INR\n    Expenses:Food:Hyd\n"
	os.WriteFile(filepath.Join(journalDir, "auto-import.ledger"), []byte(journal), 0644)

	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: journalDir, MaxCards: 10}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.confirms) != 1 {
		t.Fatalf("confirms: %v", bot.confirms)
	}
	if !strings.Contains(bot.confirms[0], "Dup Spend") {
		t.Fatalf("confirm text: %q", bot.confirms[0])
	}
	d.mu.Lock()
	n := len(d.pendingRemovals)
	d.mu.Unlock()
	if n != 1 {
		t.Fatalf("pendingRemovals: %+v", d.pendingRemovals)
	}
}

func TestHandleCCStatementExtraUnlocatableReportedInSummary(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 1000,
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		{Date: day("2026-06-22"), Account: cardAcct, Amount: -999.00, Payee: "Ghost"},
	}}
	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: t.TempDir(), MaxCards: 10} // empty journal dir -> FindEntry ErrNotFound
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.confirms) != 0 {
		t.Fatalf("expected no confirm card, got %v", bot.confirms)
	}
	if !strings.Contains(bot.texts[0], "unlocatable") {
		t.Fatalf("summary must mention unlocatable extra: %q", bot.texts[0])
	}
}

func TestHandleCallbackRemoveConfirmed(t *testing.T) {
	journalDir := t.TempDir()
	keep := "2026/06/20 Coffee\n    " + cardAcct + "               -240.00 INR\n    Expenses:Food:Hyd\n"
	dup := "2026/06/22 Dup\n    " + cardAcct + "               -999.00 INR\n    Expenses:Food:Hyd\n"
	path := filepath.Join(journalDir, "auto-import.ledger")
	os.WriteFile(path, []byte(keep+"\n"+dup), 0644)

	client := &fakeClient{}
	bot := &fakeBot{}
	d := &Deps{Client: client, Bot: bot, JournalDir: journalDir}
	d.setPendingRemoval(42, pendingRemoval{block: dup, file: path})

	handled, err := d.HandleCallback("ccdel", 42)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "Dup") {
		t.Fatalf("block not removed: %q", after)
	}
	if !strings.Contains(string(after), "Coffee") {
		t.Fatalf("kept entry must survive: %q", after)
	}
	if client.syncCalls != 1 {
		t.Fatalf("SyncJournal not called: %d", client.syncCalls)
	}
	if !strings.Contains(bot.edits[42], "removed") {
		t.Fatalf("edit message: %q", bot.edits[42])
	}
	baks, _ := filepath.Glob(path + ".*.bak")
	if len(baks) != 1 {
		t.Fatal("backup missing")
	}
}

func TestHandleCallbackKeepLeavesFileUntouched(t *testing.T) {
	journalDir := t.TempDir()
	dup := "2026/06/22 Dup\n    " + cardAcct + "               -999.00 INR\n    Expenses:Food:Hyd\n"
	path := filepath.Join(journalDir, "auto-import.ledger")
	os.WriteFile(path, []byte(dup), 0644)

	client := &fakeClient{}
	bot := &fakeBot{}
	d := &Deps{Client: client, Bot: bot, JournalDir: journalDir}
	d.setPendingRemoval(7, pendingRemoval{block: dup, file: path})

	handled, err := d.HandleCallback("cckeep", 7)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "Dup") {
		t.Fatalf("block should remain untouched: %q", after)
	}
	if client.syncCalls != 0 {
		t.Fatalf("SyncJournal must not be called on keep")
	}
	if !strings.Contains(bot.edits[7], "kept") {
		t.Fatalf("edit message: %q", bot.edits[7])
	}
}

func TestHandleCallbackUnknownDataNotHandled(t *testing.T) {
	d := &Deps{}
	handled, err := d.HandleCallback("approve", 1)
	if handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}

// TestHandleCallbackStaleMessageIDIgnored guards against a restart or a
// re-delivered callback: if there's no pendingRemoval for this messageID, the
// callback must be a no-op on the journal — never fall back to editing an
// unrelated file/block.
func TestHandleCallbackStaleMessageIDIgnored(t *testing.T) {
	journalDir := t.TempDir()
	dup := "2026/06/22 Dup\n    L:C               -999.00 INR\n    E:F\n"
	path := filepath.Join(journalDir, "auto-import.ledger")
	os.WriteFile(path, []byte(dup), 0644)
	client := &fakeClient{}
	bot := &fakeBot{}
	d := &Deps{Client: client, Bot: bot, JournalDir: journalDir}

	handled, err := d.HandleCallback("ccdel", 99)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if client.syncCalls != 0 {
		t.Fatalf("must not sync for unknown messageID")
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "Dup") {
		t.Fatalf("file must be untouched for stale id")
	}
}
