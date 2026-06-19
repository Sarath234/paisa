// internal/agent/gmail/client.go
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleGmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Message is a Gmail message with its attachment.
type Message struct {
	ID      string
	Subject string
}

// Client wraps the Gmail API service.
type Client struct {
	svc       *googleGmail.Service
	oauthConf *oauth2.Config
	tokenFile string
}

// New creates a Client using credentials from cfg.
// Returns (nil, nil) if clientID is empty (Gmail not configured).
func New(clientID, clientSecret, tokenFile string) (*Client, error) {
	if clientID == "" {
		return nil, nil
	}
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes: []string{
			googleGmail.GmailReadonlyScope,
			"https://www.googleapis.com/auth/gmail.modify", // for MarkRead
		},
		Endpoint:    google.Endpoint,
		RedirectURL: "http://localhost:8787/oauth/callback",
	}
	tok, err := loadToken(tokenFile)
	if err != nil {
		return &Client{oauthConf: conf, tokenFile: tokenFile}, nil // needs auth
	}
	svc, err := newService(conf, tok)
	if err != nil {
		return nil, fmt.Errorf("gmail: new service: %w", err)
	}
	return &Client{svc: svc, oauthConf: conf, tokenFile: tokenFile}, nil
}

// IsAuthorized returns true when a valid token is loaded.
func (c *Client) IsAuthorized() bool { return c.svc != nil }

// AuthURL returns the URL the user must open to authorise Gmail access.
func (c *Client) AuthURL() string {
	return c.oauthConf.AuthCodeURL("state", oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges an OAuth2 auth code for a token and saves it.
func (c *Client) ExchangeCode(code string) error {
	tok, err := c.oauthConf.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("gmail: exchange code: %w", err)
	}
	if err := saveToken(c.tokenFile, tok); err != nil {
		return fmt.Errorf("gmail: save token: %w", err)
	}
	svc, err := newService(c.oauthConf, tok)
	if err != nil {
		return fmt.Errorf("gmail: new service after exchange: %w", err)
	}
	c.svc = svc
	return nil
}

// Search returns unread messages whose subject contains subjectMatch.
func (c *Client) Search(subjectMatch string) ([]Message, error) {
	q := fmt.Sprintf("is:unread subject:%s has:attachment", subjectMatch)
	r, err := c.svc.Users.Messages.List("me").Q(q).MaxResults(10).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: list: %w", err)
	}
	var msgs []Message
	for _, m := range r.Messages {
		full, err := c.svc.Users.Messages.Get("me", m.Id).Format("metadata").
			MetadataHeaders("Subject").Do()
		if err != nil {
			continue
		}
		subject := ""
		for _, h := range full.Payload.Headers {
			if h.Name == "Subject" {
				subject = h.Value
				break
			}
		}
		msgs = append(msgs, Message{ID: m.Id, Subject: subject})
	}
	return msgs, nil
}

// DownloadPDF downloads the first PDF attachment from the message and returns its bytes.
func (c *Client) DownloadPDF(msgID string) ([]byte, error) {
	msg, err := c.svc.Users.Messages.Get("me", msgID).Format("full").Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: get message: %w", err)
	}
	for _, part := range msg.Payload.Parts {
		if part.MimeType != "application/pdf" {
			continue
		}
		att, err := c.svc.Users.Messages.Attachments.Get("me", msgID, part.Body.AttachmentId).Do()
		if err != nil {
			return nil, fmt.Errorf("gmail: get attachment: %w", err)
		}
		// Gmail returns base64url-encoded data in the Data field.
		data, err := decodeAttachmentData(att.Data)
		if err != nil {
			return nil, fmt.Errorf("gmail: decode attachment: %w", err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("gmail: no PDF attachment in message %s", msgID)
}

// MarkRead removes the UNREAD label from a message.
func (c *Client) MarkRead(msgID string) error {
	req := &googleGmail.ModifyMessageRequest{RemoveLabelIds: []string{"UNREAD"}}
	_, err := c.svc.Users.Messages.Modify("me", msgID, req).Do()
	return err
}

// OAuthCallbackServer starts a local HTTP server on port 8787, waits for the
// OAuth callback, and returns the auth code. It times out after 5 minutes.
func OAuthCallbackServer() (string, error) {
	codeCh := make(chan string, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":8787", Handler: mux}
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "Gmail authorised — you can close this tab.")
		codeCh <- code
	})
	go srv.ListenAndServe()                  //nolint:errcheck
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	select {
	case code := <-codeCh:
		return code, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("oauth: timed out waiting for callback")
	}
}

func decodeAttachmentData(data string) ([]byte, error) {
	// Gmail returns base64url-encoded data; add padding if needed.
	data = strings.ReplaceAll(data, "-", "+")
	data = strings.ReplaceAll(data, "_", "/")
	// Add padding if needed.
	for len(data)%4 != 0 {
		data += "="
	}
	return base64.StdEncoding.DecodeString(data)
}

func newService(conf *oauth2.Config, tok *oauth2.Token) (*googleGmail.Service, error) {
	ctx := context.Background()
	ts := conf.TokenSource(ctx, tok)
	return googleGmail.NewService(ctx, option.WithTokenSource(ts))
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
