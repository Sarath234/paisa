package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ananthakumaran/paisa/internal/agent/paisaclient"
)

func utilCard(balance, limit float64) paisaclient.CreditCardSummary {
	return paisaclient.CreditCardSummary{
		Account:     "Liabilities:CreditCard:Axis",
		Balance:     balance,
		CreditLimit: limit,
	}
}

func TestCCUtilizationHighestBandOnly(t *testing.T) {
	cases := []struct {
		balance  float64
		wantBand string // "" = no insight
	}{
		{30000, ""},
		{55000, "/50/"},
		{78000, "/75/"},
		{95000, "/90/"},
	}
	for _, c := range cases {
		m := NewCCUtilization(&fakeFetcher{cards: []paisaclient.CreditCardSummary{utilCard(c.balance, 100000)}}, []int{50, 75, 90}, 8)
		m.Now = func() time.Time { return day("2026-07-15").Add(8 * time.Hour) }
		insights, err := m.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if c.wantBand == "" {
			if len(insights) != 0 {
				t.Errorf("balance %.0f: want quiet, got %+v", c.balance, insights)
			}
			continue
		}
		if len(insights) != 1 {
			t.Fatalf("balance %.0f: got %d insights", c.balance, len(insights))
		}
		in := insights[0]
		if !strings.Contains(in.Key, c.wantBand) || !strings.HasSuffix(in.Key, "/2026-07") {
			t.Errorf("balance %.0f: key %q", c.balance, in.Key)
		}
		if in.Urgency != Digest {
			t.Error("want Digest")
		}
	}
}

func TestCCUtilizationMessageAndZeroLimit(t *testing.T) {
	m := NewCCUtilization(&fakeFetcher{cards: []paisaclient.CreditCardSummary{utilCard(78000, 100000)}}, []int{50, 75, 90}, 8)
	m.Now = func() time.Time { return day("2026-07-15") }
	insights, _ := m.Check(context.Background())
	title := insights[0].Title
	for _, want := range []string{"Axis", "78%", "₹78000.00", "₹100000.00"} {
		if !strings.Contains(title, want) {
			t.Errorf("title missing %q: %q", want, title)
		}
	}

	// creditLimit == 0 must not divide by zero
	m = NewCCUtilization(&fakeFetcher{cards: []paisaclient.CreditCardSummary{utilCard(5000, 0)}}, []int{50}, 8)
	m.Now = func() time.Time { return day("2026-07-15") }
	insights, err := m.Check(context.Background())
	if err != nil || len(insights) != 0 {
		t.Fatalf("zero limit: %v %+v", err, insights)
	}
}
