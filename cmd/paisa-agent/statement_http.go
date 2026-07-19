// cmd/paisa-agent/statement_http.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const maxUploadBytes = 15 * 1024 * 1024
const uploadBackdate = 31 * time.Second

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._ -]`)

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// statementUploadHandler accepts a multipart PDF and drops it into dropDir —
// the drop-folder poller does everything else. ModTime is backdated past the
// poller's settle guard (we completed the write), then kick() forces a scan.
func statementUploadHandler(dropDir string, kick func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		if dropDir == "" {
			writeJSONError(w, http.StatusServiceUnavailable, "statements.drop_dir not configured")
			return
		}
		if err := r.ParseMultipartForm(maxUploadBytes + 1024); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()
		if !strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
			writeJSONError(w, http.StatusBadRequest, "only .pdf files are accepted")
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "read upload: "+err.Error())
			return
		}
		if len(data) == 0 {
			writeJSONError(w, http.StatusBadRequest, "empty file")
			return
		}
		if len(data) > maxUploadBytes {
			writeJSONError(w, http.StatusBadRequest, "file exceeds 15 MB limit")
			return
		}

		name := sanitizeFilename(header.Filename)
		path, name := collisionFreePath(dropDir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "store upload: "+err.Error())
			return
		}
		past := time.Now().Add(-uploadBackdate)
		if err := os.Chtimes(path, past, past); err != nil {
			// Non-fatal: file just waits out MinAge on the regular tick.
			log.Warnf("statement upload: backdate %s: %v", path, err)
		}
		kick()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"file": name})
	}
}

func sanitizeFilename(raw string) string {
	base := filepath.Base(raw)
	base = unsafeFilenameChars.ReplaceAllString(base, "_")
	if base == "" || base == "." || base == ".." {
		base = "statement.pdf"
	}
	return base
}

// collisionFreePath appends -1, -2… before the extension until unused.
func collisionFreePath(dir, name string) (string, string) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
}
