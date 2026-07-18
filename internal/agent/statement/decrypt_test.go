// internal/agent/statement/decrypt_test.go
package statement

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// makeEncryptedPDF builds a minimal one-page PDF, encrypts it with pw.
func makeEncryptedPDF(t *testing.T, pw string) []byte {
	t.Helper()
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.pdf")
	// minimal valid PDF
	minimal := "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\nxref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000052 00000 n \n0000000101 00000 n \ntrailer<</Size 4/Root 1 0 R>>\nstartxref\n164\n%%EOF"
	if err := os.WriteFile(plain, []byte(minimal), 0644); err != nil {
		t.Fatal(err)
	}
	enc := filepath.Join(dir, "enc.pdf")
	conf := model.NewAESConfiguration(pw, pw, 256)
	if err := api.EncryptFile(plain, enc, conf); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecryptWithCorrectPassword(t *testing.T) {
	enc := makeEncryptedPDF(t, "secret123")
	out, err := Decrypt(enc, "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("decrypted output is not a PDF")
	}
}

func TestDecryptWrongPasswordErrors(t *testing.T) {
	enc := makeEncryptedPDF(t, "secret123")
	_, err := Decrypt(enc, "wrong")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "password") {
		t.Fatalf("want password error, got %v", err)
	}
}

func TestDecryptUnencryptedPassthrough(t *testing.T) {
	plain := []byte("%PDF-1.4 not encrypted")
	out, err := Decrypt(plain, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatal("unencrypted input must pass through unchanged")
	}
}
