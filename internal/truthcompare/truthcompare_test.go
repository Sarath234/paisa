package truthcompare

import (
	"testing"
	"time"
)

func TestAuthorityValuesArePinned(t *testing.T) {
	if AuthorityAPI != 0 || AuthoritySMS != 1 || AuthorityPDF != 2 {
		t.Fatalf("Authority values reordered: api=%d sms=%d pdf=%d", AuthorityAPI, AuthoritySMS, AuthorityPDF)
	}
}

func TestWithinWindow(t *testing.T) {
	base := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		a, b   time.Time
		window time.Duration
		want   bool
	}{
		{"exact match", base, base, 3 * 24 * time.Hour, true},
		{"within window", base, base.Add(2 * 24 * time.Hour), 3 * 24 * time.Hour, true},
		{"exactly at boundary is inclusive", base, base.Add(3 * 24 * time.Hour), 3 * 24 * time.Hour, true},
		{"just outside boundary", base, base.Add(3*24*time.Hour + time.Second), 3 * 24 * time.Hour, false},
		{"b before a within window", base.Add(2 * 24 * time.Hour), base, 3 * 24 * time.Hour, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WithinWindow(c.a, c.b, c.window); got != c.want {
				t.Errorf("WithinWindow(%v, %v, %v) = %v, want %v", c.a, c.b, c.window, got, c.want)
			}
		})
	}
}

func TestSameDay(t *testing.T) {
	d1 := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 7, 10, 23, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	if !SameDay(d1, d2) {
		t.Error("expected same calendar day")
	}
	if SameDay(d1, d3) {
		t.Error("expected different calendar days")
	}
}

func TestFieldStatus(t *testing.T) {
	cases := []struct {
		name      string
		authority Authority
		agrees    bool
		want      Status
	}{
		{"below SMS authority, agrees, still computed", AuthorityAPI, true, StatusComputed},
		{"below SMS authority, disagrees, still computed", AuthorityAPI, false, StatusComputed},
		{"SMS authority, agrees, confirmed", AuthoritySMS, true, StatusConfirmed},
		{"SMS authority, disagrees, corrected", AuthoritySMS, false, StatusCorrected},
		{"PDF authority, agrees, confirmed", AuthorityPDF, true, StatusConfirmed},
		{"PDF authority, disagrees, corrected", AuthorityPDF, false, StatusCorrected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FieldStatus(c.authority, c.agrees); got != c.want {
				t.Errorf("FieldStatus(%v, %v) = %v, want %v", c.authority, c.agrees, got, c.want)
			}
		})
	}
}

func TestChannelLabel(t *testing.T) {
	if ChannelLabel(AuthorityPDF) != "pdf" {
		t.Error("want pdf for AuthorityPDF")
	}
	if ChannelLabel(AuthoritySMS) != "sms" {
		t.Error("want sms for AuthoritySMS")
	}
	if ChannelLabel(AuthorityAPI) != "sms" {
		t.Error("want sms default for AuthorityAPI")
	}
}
