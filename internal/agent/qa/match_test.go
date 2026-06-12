// internal/agent/qa/match_test.go
package qa

import (
	"reflect"
	"testing"
)

func TestMatchAccounts(t *testing.T) {
	accounts := []string{
		"Expenses:Food", "Expenses:Food:Hyd", "Expenses:Groceries",
		"Assets:Checking:Axis", "Assets:Checking:HDFC",
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"food", []string{"Expenses:Food"}},                                    // exact leaf beats substring
		{"Expenses:Food:Hyd", []string{"Expenses:Food:Hyd"}},                   // exact full path
		{"FOOD", []string{"Expenses:Food"}},                                    // case-insensitive
		{"checking", []string{"Assets:Checking:Axis", "Assets:Checking:HDFC"}}, // substring, multiple
		{"axis", []string{"Assets:Checking:Axis"}},                             // exact leaf
		{"travel", nil},                                                        // no match
		{"", nil},                                                              // empty query
	}
	for _, c := range cases {
		got := MatchAccounts(c.query, accounts)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MatchAccounts(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}
