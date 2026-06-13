// internal/agent/rulelearning/edit_test.go
package rulelearning

import (
	"strings"
	"testing"
)

func TestFormatEditTemplate(t *testing.T) {
	r := PendingRule{Keyword: "swiggy", Account: "Expenses:Food:Hyd", Description: "Food Swiggy"}
	got := FormatEditTemplate(r)
	if !strings.Contains(got, "keyword: swiggy") {
		t.Errorf("missing keyword line in:\n%s", got)
	}
	if !strings.Contains(got, "account: Expenses:Food:Hyd") {
		t.Errorf("missing account line in:\n%s", got)
	}
	if !strings.Contains(got, "description: Food Swiggy") {
		t.Errorf("missing description line in:\n%s", got)
	}
}

func TestParseEditReply(t *testing.T) {
	existing := PendingRule{
		Keyword:     "food swiggy",
		Account:     "Expenses:Food:Generic",
		Description: "Food Swiggy",
	}
	tests := []struct {
		name     string
		input    string
		wantKw   string
		wantAcc  string
		wantDesc string
	}{
		{
			name:     "keyword corrected",
			input:    "keyword: swiggy\naccount: Expenses:Food:Generic\ndescription: Food Swiggy",
			wantKw:   "swiggy",
			wantAcc:  "Expenses:Food:Generic",
			wantDesc: "Food Swiggy",
		},
		{
			name:     "account corrected",
			input:    "keyword: food swiggy\naccount: Expenses:Food:Hyd\ndescription: Food Swiggy",
			wantKw:   "food swiggy",
			wantAcc:  "Expenses:Food:Hyd",
			wantDesc: "Food Swiggy",
		},
		{
			name:     "unknown keys ignored, known keys applied",
			input:    "random: ignored\nkeyword: swiggy",
			wantKw:   "swiggy",
			wantAcc:  "Expenses:Food:Generic",
			wantDesc: "Food Swiggy",
		},
		{
			name:     "no recognised keys returns existing unchanged",
			input:    "this is not a key-value line",
			wantKw:   "food swiggy",
			wantAcc:  "Expenses:Food:Generic",
			wantDesc: "Food Swiggy",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseEditReply(tc.input, existing)
			if got.Keyword != tc.wantKw {
				t.Errorf("Keyword=%q want %q", got.Keyword, tc.wantKw)
			}
			if got.Account != tc.wantAcc {
				t.Errorf("Account=%q want %q", got.Account, tc.wantAcc)
			}
			if got.Description != tc.wantDesc {
				t.Errorf("Description=%q want %q", got.Description, tc.wantDesc)
			}
		})
	}
}
