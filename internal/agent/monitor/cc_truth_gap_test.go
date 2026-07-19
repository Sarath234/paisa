package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/billtruth"
	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

func TestCCTruthGapNudges(t *testing.T) {
	api := &fakeFetcher{cards: []paisaclient.CreditCardSummary{{
		Account: "Liabilities:CreditCard:HDFC2527",
		Bills:   []paisaclient.CreditCardBill{{StatementEndDate: day("2026-07-14"), DueDate: day("2026-08-05")}},
	}}}
	empty, _ := billtruth.Open(t.TempDir())
	m := NewCCTruthGap(empty, api, 3, 8)
	m.Now = func() time.Time { return day("2026-07-18").Add(8 * time.Hour) }
	insights, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].Key != "cc-truth-gap/Liabilities:CreditCard:HDFC2527/2026-07-14" {
		t.Fatalf("%+v", insights)
	}
	if insights[0].Urgency != Digest {
		t.Error("truth-gap nudges are digest, not immediate")
	}
}

func TestCCTruthGapSilentWhenTruthArrived(t *testing.T) {
	api := &fakeFetcher{cards: []paisaclient.CreditCardSummary{{
		Account: "Liabilities:CreditCard:HDFC2527",
		Bills:   []paisaclient.CreditCardBill{{StatementEndDate: day("2026-07-14")}},
	}}}
	s := truthStore(t, billtruth.Bill{
		Account:   "Liabilities:CreditCard:HDFC2527",
		PeriodEnd: day("2026-07-14"), DueDate: day("2026-08-05"), TotalDue: 8940.25,
	})
	m := NewCCTruthGap(s, api, 3, 8)
	m.Now = func() time.Time { return day("2026-07-18").Add(8 * time.Hour) }
	insights, _ := m.Check(context.Background())
	if len(insights) != 0 {
		t.Fatalf("%+v", insights)
	}
}

func TestCCTruthGapSilentInsideWindow(t *testing.T) {
	api := &fakeFetcher{cards: []paisaclient.CreditCardSummary{{
		Account: "Liabilities:CreditCard:HDFC2527",
		Bills:   []paisaclient.CreditCardBill{{StatementEndDate: day("2026-07-14")}},
	}}}
	empty, _ := billtruth.Open(t.TempDir())
	m := NewCCTruthGap(empty, api, 3, 8)
	m.Now = func() time.Time { return day("2026-07-16").Add(8 * time.Hour) } // 2 days
	insights, _ := m.Check(context.Background())
	if len(insights) != 0 {
		t.Fatalf("%+v", insights)
	}
}
