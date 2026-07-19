package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadProxyStreamsMultipart(t *testing.T) {
	var gotContentType, gotBody string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/statement/upload" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"file":"a.pdf"}`))
	}))
	defer fake.Close()
	old := agentBaseURL
	agentBaseURL = fake.URL
	defer func() { agentBaseURL = old }()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "a.pdf")
	fw.Write([]byte("%PDF"))
	mw.Close()

	gin.SetMode(gin.TestMode)
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest("POST", "/api/agent/statement/upload", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())

	UploadStatement(c)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "a.pdf") {
		t.Fatalf("code %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("boundary lost: %q", gotContentType)
	}
	if !strings.Contains(gotBody, "%PDF") {
		t.Fatal("body not streamed")
	}
}

func TestStatusProxyForwardsQuery(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("file"); got != "a b.pdf" {
			t.Errorf("file param %q", got)
		}
		w.Write([]byte(`{"status":"queued"}`))
	}))
	defer fake.Close()
	old := agentBaseURL
	agentBaseURL = fake.URL
	defer func() { agentBaseURL = old }()

	gin.SetMode(gin.TestMode)
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest("GET", "/api/agent/statement/status?file=a+b.pdf", nil)

	StatementStatus(c)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "queued") {
		t.Fatalf("code %d body %s", rr.Code, rr.Body.String())
	}
}

func TestProxy502WhenAgentDown(t *testing.T) {
	old := agentBaseURL
	agentBaseURL = "http://127.0.0.1:1" // nothing listens
	defer func() { agentBaseURL = old }()

	gin.SetMode(gin.TestMode)
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)
	c.Request = httptest.NewRequest("GET", "/api/agent/statement/status?file=x.pdf", nil)
	StatementStatus(c)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("code %d", rr.Code)
	}
}
