// internal/agent/statement/decrypt.go
package statement

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Decrypt removes standard PDF encryption using password. Decrypted bytes
// stay in memory only — callers must not write them to disk. Empty password
// with an unencrypted PDF passes through unchanged.
func Decrypt(pdfBytes []byte, password string) ([]byte, error) {
	if password == "" {
		return pdfBytes, nil
	}
	conf := model.NewDefaultConfiguration()
	conf.UserPW = password
	conf.OwnerPW = password
	var out bytes.Buffer
	if err := api.Decrypt(bytes.NewReader(pdfBytes), &out, conf); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "password") ||
			strings.Contains(strings.ToLower(err.Error()), "authentication") {
			return nil, fmt.Errorf("pdf decrypt: wrong password: %w", err)
		}
		return nil, fmt.Errorf("pdf decrypt: %w", err)
	}
	return out.Bytes(), nil
}
