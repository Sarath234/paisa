package importrule

import (
	"os"
	"testing"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/stretchr/testify/assert"
)

func setupConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := dir + "/paisa.yaml"
	err := os.WriteFile(cfgPath, []byte("journal_path: /tmp/test.ledger\ndb_path: "+dir+"/test.db\n"), 0644)
	assert.NoError(t, err)
	config.SetConfigPath("")
	config.LoadConfigFile(cfgPath)
}

func TestAll_Empty(t *testing.T) {
	setupConfig(t)
	rules := All()
	assert.Empty(t, rules)
}

func TestUpsertAndAll(t *testing.T) {
	setupConfig(t)
	rule := config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"}
	Upsert(rule)
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "Swiggy", rules[0].Name)
	assert.Equal(t, "swiggy", rules[0].Match)
	assert.Equal(t, "Expenses:Food", rules[0].Account)
}

func TestUpsert_UpdateExisting(t *testing.T) {
	setupConfig(t)
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"})
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy.*food", Account: "Expenses:Food:Delivery"})
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "swiggy.*food", rules[0].Match)
	assert.Equal(t, "Expenses:Food:Delivery", rules[0].Account)
}

func TestDelete(t *testing.T) {
	setupConfig(t)
	Upsert(config.ImportRule{Name: "Swiggy", Match: "swiggy", Account: "Expenses:Food"})
	Upsert(config.ImportRule{Name: "Salary", Match: "salary", Account: "Income:Salary"})
	Delete("Swiggy")
	rules := All()
	assert.Len(t, rules, 1)
	assert.Equal(t, "Salary", rules[0].Name)
}
