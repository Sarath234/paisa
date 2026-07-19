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

	"github.com/ananthakumaran/paisa/internal/agent/dropfolder"
	"github.com/ananthakumaran/paisa/internal/agent/reconcile"
	"github.com/ananthakumaran/paisa/internal/agent/statement"
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

func seedReconcileRecord(t *testing.T, journalDir, account, period string, matched, missing, extra int) {
	t.Helper()
	diff := reconcile.Diff{Account: account, Matched: matched}
	for i := 0; i < missing; i++ {
		diff.Missing = append(diff.Missing, statement.Transaction{Description: fmt.Sprintf("m%d", i)})
	}
	for i := 0; i < extra; i++ {
		diff.Extra = append(diff.Extra, reconcile.LedgerEntry{Description: fmt.Sprintf("e%d", i)})
	}
	if err := reconcile.Write(journalDir, reconcile.Record{
		Period: period, GeneratedAt: time.Now(), Diff: diff,
	}); err != nil {
		t.Fatal(err)
	}
}

func statusReq(file string) *http.Request {
	return httptest.NewRequest("GET", "/statement/status?file="+file, nil)
}

func TestStatusQueuedDoneFailedUnknown(t *testing.T) {
	drop := t.TempDir()
	journal := t.TempDir()
	matches := []dropfolder.AccountMatch{{Pattern: "*6009*", LedgerAccount: "Liabilities:CreditCard:ICIC6009", Kind: "credit_card"}}
	h := statementStatusHandler(drop, matches, journal)

	os.WriteFile(filepath.Join(drop, "q-6009.pdf"), []byte("%PDF"), 0644)
	os.MkdirAll(filepath.Join(drop, "processed"), 0755)
	os.WriteFile(filepath.Join(drop, "processed", "d-6009.pdf"), []byte("%PDF"), 0644)
	os.MkdirAll(filepath.Join(drop, "failed"), 0755)
	os.WriteFile(filepath.Join(drop, "failed", "f-6009.pdf"), []byte("%PDF"), 0644)
	seedReconcileRecord(t, journal, "Liabilities:CreditCard:ICIC6009", "2026-07", 34, 2, 1)

	cases := map[string]string{
		"q-6009.pdf": "queued", "d-6009.pdf": "done", "f-6009.pdf": "failed", "nope.pdf": "unknown",
	}
	for file, want := range cases {
		rr := httptest.NewRecorder()
		h(rr, statusReq(file))
		var resp struct {
			Status  string `json:"status"`
			Summary *struct {
				Matched, Missing, Extra int
				Period                  string
			} `json:"summary"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: %v (%s)", file, err, rr.Body.String())
		}
		if resp.Status != want {
			t.Errorf("%s: status %q want %q", file, resp.Status, want)
		}
		if want == "done" {
			if resp.Summary == nil || resp.Summary.Matched != 34 || resp.Summary.Missing != 2 || resp.Summary.Extra != 1 || resp.Summary.Period != "2026-07" {
				t.Errorf("done summary: %+v", resp.Summary)
			}
		} else if resp.Summary != nil {
			t.Errorf("%s: unexpected summary", file)
		}
	}
}

func TestStatusDoneWithoutRecordOmitsSummary(t *testing.T) {
	drop := t.TempDir()
	os.MkdirAll(filepath.Join(drop, "processed"), 0755)
	os.WriteFile(filepath.Join(drop, "processed", "d-6009.pdf"), []byte("%PDF"), 0644)
	h := statementStatusHandler(drop, []dropfolder.AccountMatch{{Pattern: "*6009*", LedgerAccount: "L:CC:X"}}, t.TempDir())
	rr := httptest.NewRecorder()
	h(rr, statusReq("d-6009.pdf"))
	if !strings.Contains(rr.Body.String(), `"done"`) || strings.Contains(rr.Body.String(), "summary") {
		t.Fatalf("body: %s", rr.Body.String())
	}
}

func TestStatusRejectsPaths(t *testing.T) {
	h := statementStatusHandler(t.TempDir(), nil, t.TempDir())
	for _, bad := range []string{"", "a/b.pdf", "..%2Fx.pdf"} {
		rr := httptest.NewRecorder()
		h(rr, statusReq(bad))
		if rr.Code != 400 {
			t.Errorf("%q: code %d", bad, rr.Code)
		}
	}
}

func TestStatusNameCollisionNewestWins(t *testing.T) {
	drop := t.TempDir()
	os.MkdirAll(filepath.Join(drop, "processed"), 0755)
	os.MkdirAll(filepath.Join(drop, "failed"), 0755)
	processed := filepath.Join(drop, "processed", "x.pdf")
	failed := filepath.Join(drop, "failed", "x.pdf")
	os.WriteFile(processed, []byte("%PDF"), 0644)
	os.WriteFile(failed, []byte("%PDF"), 0644)

	h := statementStatusHandler(drop, nil, t.TempDir())
	base := time.Now().Add(-time.Hour)

	// failed/ is newer than processed/ → re-upload failed after an old success.
	if err := os.Chtimes(processed, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(failed, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h(rr, statusReq("x.pdf"))
	if !strings.Contains(rr.Body.String(), `"failed"`) {
		t.Fatalf("failed newer: body %s", rr.Body.String())
	}

	// processed/ is newer than failed/ → old failure, later success.
	if err := os.Chtimes(processed, base.Add(2*time.Minute), base.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	h(rr, statusReq("x.pdf"))
	if !strings.Contains(rr.Body.String(), `"done"`) {
		t.Fatalf("processed newer: body %s", rr.Body.String())
	}
}

func TestStatusUnconfigured503(t *testing.T) {
	h := statementStatusHandler("", nil, t.TempDir())
	rr := httptest.NewRecorder()
	h(rr, statusReq("x.pdf"))
	if rr.Code != 503 {
		t.Fatalf("code %d", rr.Code)
	}
}
