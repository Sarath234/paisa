// internal/agent/rulelearning/derive_test.go
package rulelearning

import (
	"testing"

	agentledger "github.com/ananthakumaran/paisa/internal/agent/ledger"
)

func TestDerive(t *testing.T) {
	tests := []struct {
		name        string
		original    agentledger.Entry
		corrected   agentledger.Entry
		wantKeyword string
		wantAccount string
		wantDesc    string
		wantOK      bool
	}{
		{
			name:        "dest changed",
			original:    agentledger.Entry{Desc: "Swiggy", Dest: "Expenses:Food:Generic"},
			corrected:   agentledger.Entry{Desc: "Food Swiggy", Dest: "Expenses:Food:Hyd"},
			wantKeyword: "swiggy",
			wantAccount: "Expenses:Food:Hyd",
			wantDesc:    "Food Swiggy",
			wantOK:      true,
		},
		{
			name:      "dest unchanged — no rule",
			original:  agentledger.Entry{Desc: "Swiggy", Dest: "Expenses:Food:Hyd"},
			corrected: agentledger.Entry{Desc: "Food Swiggy", Dest: "Expenses:Food:Hyd"},
			wantOK:    false,
		},
		{
			name:      "empty original desc — no keyword possible",
			original:  agentledger.Entry{Desc: "", Dest: "Expenses:Food:Generic"},
			corrected: agentledger.Entry{Desc: "Food Swiggy", Dest: "Expenses:Food:Hyd"},
			wantOK:    false,
		},
		{
			name:        "keyword lowercased and trimmed",
			original:    agentledger.Entry{Desc: "  SREE TEJA  ", Dest: "Expenses:Food:Generic"},
			corrected:   agentledger.Entry{Desc: "Tea @ SREE TEJA", Dest: "Expenses:Food:Hyd"},
			wantKeyword: "sree teja",
			wantAccount: "Expenses:Food:Hyd",
			wantDesc:    "Tea @ SREE TEJA",
			wantOK:      true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kw, acc, desc, ok := Derive(tc.original, tc.corrected)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if kw != tc.wantKeyword {
				t.Errorf("keyword=%q want %q", kw, tc.wantKeyword)
			}
			if acc != tc.wantAccount {
				t.Errorf("account=%q want %q", acc, tc.wantAccount)
			}
			if desc != tc.wantDesc {
				t.Errorf("description=%q want %q", desc, tc.wantDesc)
			}
		})
	}
}
