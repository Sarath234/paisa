// internal/agent/ccrecon/ccrecon_test.go
package ccrecon

import (
	"fmt"
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
	texts, drafts []string
	nextID        int
}

func (f *fakeBot) SendText(s string) error { f.texts = append(f.texts, s); return nil }
func (f *fakeBot) SendDraft(s string) (int, error) {
	f.drafts = append(f.drafts, s)
	f.nextID++
	return f.nextID, nil
}

type fakeClient struct{ postings []paisaclient.Posting }

func (f *fakeClient) Postings() ([]paisaclient.Posting, error) { return f.postings, nil }
func (f *fakeClient) SyncJournal() error                       { return nil }

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
