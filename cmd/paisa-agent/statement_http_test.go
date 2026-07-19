// cmd/paisa-agent/statement_http_test.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func uploadReq(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	w.Close()
	req := httptest.NewRequest("POST", "/statement/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestUploadHappyPath(t *testing.T) {
	dir := t.TempDir()
	kicked := false
	h := statementUploadHandler(dir, func() { kicked = true })
	rr := httptest.NewRecorder()
	h(rr, uploadReq(t, "stmt-6009.pdf", []byte("%PDF-1.4 data")))
	if rr.Code != 200 {
		t.Fatalf("code %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct{ File string }
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.File != "stmt-6009.pdf" {
		t.Fatalf("file: %q", resp.File)
	}
	path := filepath.Join(dir, resp.File)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(info.ModTime()); age < 30*time.Second {
		t.Fatalf("ModTime must be backdated ≥31s, age %v", age)
	}
	if !kicked {
		t.Fatal("poller not kicked")
	}
}

func TestUploadRejectsNonPDF(t *testing.T) {
	h := statementUploadHandler(t.TempDir(), func() {})
	rr := httptest.NewRecorder()
	h(rr, uploadReq(t, "notes.txt", []byte("x")))
	if rr.Code != 400 {
		t.Fatalf("code %d", rr.Code)
	}
}

func TestUploadRejectsEmptyAndOversize(t *testing.T) {
	h := statementUploadHandler(t.TempDir(), func() {})
	rr := httptest.NewRecorder()
	h(rr, uploadReq(t, "a.pdf", nil))
	if rr.Code != 400 {
		t.Fatalf("empty: code %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h(rr, uploadReq(t, "big.pdf", bytes.Repeat([]byte("x"), 15*1024*1024+1)))
	if rr.Code != 400 {
		t.Fatalf("oversize: code %d", rr.Code)
	}
}

func TestUploadSanitizesAndSuffixesCollisions(t *testing.T) {
	dir := t.TempDir()
	h := statementUploadHandler(dir, func() {})
	rr := httptest.NewRecorder()
	h(rr, uploadReq(t, "../we ird$$name.pdf", []byte("%PDF")))
	var r1 struct{ File string }
	json.Unmarshal(rr.Body.Bytes(), &r1)
	if strings.ContainsAny(r1.File, "/\\$") {
		t.Fatalf("unsanitized: %q", r1.File)
	}
	if !strings.HasSuffix(r1.File, ".pdf") {
		t.Fatalf("extension lost: %q", r1.File)
	}
	// same name again → suffixed
	rr = httptest.NewRecorder()
	h(rr, uploadReq(t, "../we ird$$name.pdf", []byte("%PDF")))
	var r2 struct{ File string }
	json.Unmarshal(rr.Body.Bytes(), &r2)
	if r2.File == r1.File {
		t.Fatal("collision not suffixed")
	}
	if _, err := os.Stat(filepath.Join(dir, r2.File)); err != nil {
		t.Fatal(err)
	}
}

func TestUploadConcurrentSameNameNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	h := statementUploadHandler(dir, func() {})

	const n = 20
	var wg sync.WaitGroup
	codes := make([]int, n)
	names := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			h(rr, uploadReq(t, "same.pdf", []byte(fmt.Sprintf("%%PDF payload %d", i))))
			codes[i] = rr.Code
			var resp struct{ File string }
			json.Unmarshal(rr.Body.Bytes(), &resp)
			names[i] = resp.File
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if codes[i] != 200 {
			t.Fatalf("upload %d: code %d", i, codes[i])
		}
		if seen[names[i]] {
			t.Fatalf("duplicate stored name returned: %q", names[i])
		}
		seen[names[i]] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != n {
		t.Fatalf("expected %d files, found %d — collision overwrote uploads", n, len(entries))
	}
}

func TestUploadUnconfigured503(t *testing.T) {
	h := statementUploadHandler("", func() {})
	rr := httptest.NewRecorder()
	h(rr, uploadReq(t, "a.pdf", []byte("%PDF")))
	if rr.Code != 503 || !strings.Contains(rr.Body.String(), "not configured") {
		t.Fatalf("code %d body %s", rr.Code, rr.Body.String())
	}
}

func TestUploadMethodNotAllowed(t *testing.T) {
	h := statementUploadHandler(t.TempDir(), func() {})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest("GET", "/statement/upload", nil))
	if rr.Code != 405 {
		t.Fatalf("code %d", rr.Code)
	}
}
