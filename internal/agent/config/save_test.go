// internal/agent/config/save_test.go
package config

import (
	"os"
	"strings"
	"testing"
)

const yamlFixture = `paisa:
  url: http://localhost:7500

parser_rules:
  merchants:
    # Food
    - keyword: "swiggy"
      account: "Expenses:Food:Hyd"
      description: "Food Swiggy"
    - keyword: "zomato"
      account: "Expenses:Food:Hyd"
      description: "Food Zomato"
`

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "paisa-agent-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestPrependMerchantRule_insertsAtTop(t *testing.T) {
	path := writeTmp(t, yamlFixture)
	rule := MerchantRule{Keyword: "blinkit", Account: "Expenses:Groceries:Hyd", Description: "Groceries Blink"}

	if err := PrependMerchantRule(path, rule); err != nil {
		t.Fatalf("PrependMerchantRule: %v", err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)

	idxNew := strings.Index(got, `keyword: "blinkit"`)
	idxOld := strings.Index(got, `keyword: "swiggy"`)
	if idxNew < 0 {
		t.Fatal("new keyword not found in output")
	}
	if idxOld < 0 {
		t.Fatal("existing keyword missing from output")
	}
	if idxNew > idxOld {
		t.Error("new rule must appear before existing rules")
	}
	if !strings.Contains(got, "# Food") {
		t.Error("existing comment must be preserved")
	}
	if !strings.Contains(got, `account: "Expenses:Groceries:Hyd"`) {
		t.Error("account field missing")
	}
	if !strings.Contains(got, `description: "Groceries Blink"`) {
		t.Error("description field missing")
	}
}

func TestPrependMerchantRule_duplicateReturnsError(t *testing.T) {
	path := writeTmp(t, yamlFixture)
	rule := MerchantRule{Keyword: "swiggy", Account: "Expenses:Food:Hyd", Description: "Food Swiggy"}

	err := PrependMerchantRule(path, rule)
	if err != ErrDuplicateKeyword {
		t.Fatalf("want ErrDuplicateKeyword, got %v", err)
	}
}

func TestPrependMerchantRule_noMerchantsSection(t *testing.T) {
	path := writeTmp(t, "paisa:\n  url: http://localhost:7500\n")
	rule := MerchantRule{Keyword: "test", Account: "Expenses:Test", Description: "Test"}

	err := PrependMerchantRule(path, rule)
	if err == nil {
		t.Fatal("want error when merchants: section missing")
	}
}

func TestIsDuplicateKeyword(t *testing.T) {
	path := writeTmp(t, yamlFixture)

	dup, err := IsDuplicateKeyword(path, "swiggy")
	if err != nil {
		t.Fatal(err)
	}
	if !dup {
		t.Error("swiggy is in the fixture — want duplicate=true")
	}

	dup, err = IsDuplicateKeyword(path, "blinkit")
	if err != nil {
		t.Fatal(err)
	}
	if dup {
		t.Error("blinkit not in fixture — want duplicate=false")
	}
}

func TestPrependMerchantRule_specialCharsEscaped(t *testing.T) {
	path := writeTmp(t, yamlFixture)
	rule := MerchantRule{
		Keyword:     `cafe"bar`,
		Account:     `Expenses:Food:Hyd`,
		Description: `Cafe "Bar"`,
	}

	if err := PrependMerchantRule(path, rule); err != nil {
		t.Fatalf("PrependMerchantRule: %v", err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)

	if !strings.Contains(got, `keyword: "cafe\"bar"`) {
		t.Errorf("expected escaped keyword in output, got:\n%s", got)
	}
	if !strings.Contains(got, `description: "Cafe \"Bar\""`) {
		t.Errorf("expected escaped description in output, got:\n%s", got)
	}
}
