// internal/agent/config/save.go
package config

import (
	"errors"
	"os"
	"strings"
)

var ErrDuplicateKeyword = errors.New("keyword already exists in merchants list")

// IsDuplicateKeyword reports whether keyword already appears in the raw YAML at path.
func IsDuplicateKeyword(path, keyword string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), `keyword: "`+keyword+`"`), nil
}

// PrependMerchantRule inserts rule at the top of the merchants: list in the YAML
// file at path, preserving all existing content including comments.
// Returns ErrDuplicateKeyword if the keyword already exists.
func PrependMerchantRule(path string, rule MerchantRule) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, `keyword: "`+rule.Keyword+`"`) {
		return ErrDuplicateKeyword
	}

	lines := strings.Split(content, "\n")
	insertIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "merchants:" {
			insertIdx = i + 1
			break
		}
	}
	if insertIdx < 0 {
		return errors.New("merchants: section not found in config")
	}

	newLines := []string{
		`    - keyword: "` + rule.Keyword + `"`,
		`      account: "` + rule.Account + `"`,
		`      description: "` + rule.Description + `"`,
	}

	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:insertIdx]...)
	result = append(result, newLines...)
	result = append(result, lines[insertIdx:]...)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(result, "\n")), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
