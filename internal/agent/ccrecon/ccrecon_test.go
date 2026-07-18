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
	syncErr   error
}

func (f *fakeClient) Postings() ([]paisaclient.Posting, error) { return f.postings, nil }
func (f *fakeClient) SyncJournal() error                       { f.syncCalls++; return f.syncErr }

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

	// Double-tap after success must be a stale no-op: no second sync, no
	// second removal attempt against the rewritten file.
	handled, err = d.HandleCallback("ccdel", 42)
	if !handled || err != nil {
		t.Fatalf("second tap: handled=%v err=%v", handled, err)
	}
	if client.syncCalls != 1 {
		t.Fatalf("second tap must not sync again: %d", client.syncCalls)
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

// TestHandleCallbackSyncFailureSurfaced: the removal succeeded but paisa's
// journal sync failed — the card must say so, not claim a clean removal.
func TestHandleCallbackSyncFailureSurfaced(t *testing.T) {
	journalDir := t.TempDir()
	dup := "2026/06/22 Dup\n    " + cardAcct + "               -999.00 INR\n    Expenses:Food:Hyd\n"
	path := filepath.Join(journalDir, "auto-import.ledger")
	os.WriteFile(path, []byte(dup), 0644)

	client := &fakeClient{syncErr: fmt.Errorf("paisa unreachable")}
	bot := &fakeBot{}
	d := &Deps{Client: client, Bot: bot, JournalDir: journalDir}
	d.setPendingRemoval(11, pendingRemoval{block: dup, file: path})

	handled, _ := d.HandleCallback("ccdel", 11)
	if !handled {
		t.Fatal("must be handled")
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "Dup") {
		t.Fatalf("removal itself must still happen: %q", after)
	}
	if !strings.Contains(bot.edits[11], "removed") || !strings.Contains(bot.edits[11], "sync failed") {
		t.Fatalf("edit must surface sync failure: %q", bot.edits[11])
	}
}

// TestHandleCallbackRemoveFailureStaysRetryable: a failed RemoveBlock must
// leave the pending entry in place so a later tap can retry (delete-after-
// success semantics), and the card must show the error.
func TestHandleCallbackRemoveFailureStaysRetryable(t *testing.T) {
	journalDir := t.TempDir()
	dup := "2026/06/22 Dup\n    " + cardAcct + "               -999.00 INR\n    Expenses:Food:Hyd\n"
	path := filepath.Join(journalDir, "auto-import.ledger")
	// File does NOT contain the block yet -> RemoveBlock errors (0 occurrences).
	os.WriteFile(path, []byte("2026/06/20 Other\n    A:B               -1.00 INR\n    E:F\n"), 0644)

	client := &fakeClient{}
	bot := &fakeBot{}
	d := &Deps{Client: client, Bot: bot, JournalDir: journalDir}
	d.setPendingRemoval(13, pendingRemoval{block: dup, file: path})

	handled, err := d.HandleCallback("ccdel", 13)
	if !handled || err == nil {
		t.Fatalf("want handled with error, got handled=%v err=%v", handled, err)
	}
	if client.syncCalls != 0 {
		t.Fatal("must not sync after failed removal")
	}
	if !strings.Contains(bot.edits[13], "Failed") {
		t.Fatalf("edit must show failure: %q", bot.edits[13])
	}

	// The pending entry survived; once the file contains the block, a
	// second tap succeeds.
	os.WriteFile(path, []byte(dup), 0644)
	handled, err = d.HandleCallback("ccdel", 13)
	if !handled || err != nil {
		t.Fatalf("retry: handled=%v err=%v", handled, err)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "Dup") {
		t.Fatalf("retry must remove the block: %q", after)
	}
	if client.syncCalls != 1 {
		t.Fatalf("retry must sync once: %d", client.syncCalls)
	}
}

// TestHandleCCStatementExtraTruncationNoted: extras beyond the shared card
// budget get a "(N more extra)" note in the summary, like missing does.
func TestHandleCCStatementExtraTruncationNoted(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 1000,
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		{Date: day("2026-06-21"), Account: cardAcct, Amount: -111.00, Payee: "Dup One"},
		{Date: day("2026-06-22"), Account: cardAcct, Amount: -222.00, Payee: "Dup Two"},
	}}
	journalDir := t.TempDir()
	journal := "2026/06/21 Dup One\n    " + cardAcct + "               -111.00 INR\n    Expenses:Food:Hyd\n\n" +
		"2026/06/22 Dup Two\n    " + cardAcct + "               -222.00 INR\n    Expenses:Food:Hyd\n"
	os.WriteFile(filepath.Join(journalDir, "auto-import.ledger"), []byte(journal), 0644)

	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: journalDir, MaxCards: 1}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.confirms) != 1 {
		t.Fatalf("budget of 1: confirms=%v", bot.confirms)
	}
	if !strings.Contains(bot.texts[0], "1 more extra") {
		t.Fatalf("summary must note truncated extras: %q", bot.texts[0])
	}
}

// TestHandleCCStatementNextCycleSpendNotReportedAsExtra: postings are fetched
// from a window widened by ±3d so near-boundary statement transactions still
// match (see ccDateWindow). But that widened window also pulls in a
// legitimate next-cycle spend dated periodEnd+2d that's already in the
// ledger (posted before this statement's PDF even arrived) — CompareCC has
// nothing in the statement to match it against, so it comes back Extra. That
// must NOT become a "remove?" card, and must not be counted in the summary's
// extra count: it's real spend from the NEXT cycle, not a duplicate.
func TestHandleCCStatementNextCycleSpendNotReportedAsExtra(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 1000,
		// no statement transactions: nothing to match against
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		// periodEnd+2d: inside the ±3d fetch window, outside the statement period.
		{Date: day("2026-07-12"), Account: cardAcct, Amount: -500.00, Payee: "Next Cycle Spend"},
	}}
	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: t.TempDir(), MaxCards: 10}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.confirms) != 0 {
		t.Fatalf("next-cycle spend must not raise a removal card: %v", bot.confirms)
	}
	if !strings.Contains(bot.texts[0], "0 extra") {
		t.Fatalf("next-cycle spend must not be counted as extra: %q", bot.texts[0])
	}
}

// TestHandleCCStatementExtraAtPeriodEndExactlyStillExtra: the boundary itself
// (periodEnd, no offset) is inside the statement period and must still be
// treated as a genuine extra (unmatched, in-period ledger entry).
func TestHandleCCStatementExtraAtPeriodEndExactlyStillExtra(t *testing.T) {
	store, _ := billtruth.Open(t.TempDir())
	res := statement.CCResult{
		Last4: "6009", PeriodStart: day("2026-06-11"), PeriodEnd: day("2026-07-10"),
		DueDate: day("2026-07-30"), TotalDue: 1000,
	}
	client := &fakeClient{postings: []paisaclient.Posting{
		{Date: day("2026-07-10"), Account: cardAcct, Amount: -500.00, Payee: "Boundary Dup"},
	}}
	journalDir := t.TempDir()
	journal := "2026/07/10 Boundary Dup\n    " + cardAcct + "               -500.00 INR\n    Expenses:Food:Hyd\n"
	os.WriteFile(filepath.Join(journalDir, "auto-import.ledger"), []byte(journal), 0644)

	bot := &fakeBot{}
	d := &Deps{Store: store, Parsers: map[string]statement.CCParser{"icici_cc": &stubParser{res: res}},
		Client: client, Approvals: approval.NewStore(), Bot: bot,
		JournalDir: journalDir, MaxCards: 10}
	if err := d.HandleCCStatement("stmt.pdf", []byte("%PDF"), cardAcct, ""); err != nil {
		t.Fatal(err)
	}
	if len(bot.confirms) != 1 {
		t.Fatalf("periodEnd-exact extra must still raise a removal card: %v", bot.confirms)
	}
	if !strings.Contains(bot.texts[0], "1 extra") {
		t.Fatalf("periodEnd-exact extra must be counted: %q", bot.texts[0])
	}
}
