package gmail

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/api/gmail/v1"
)

type RawEmail struct {
	ID         string
	Subject    string
	Body       string    // plain text body for AlertEmail
	PDFText    string    // extracted PDF text for StatementEmail
	Type       EmailType
	ReceivedAt time.Time
}

type Poller struct {
	svc         *gmail.Service
	labels      []string
	lastChecked int64 // Unix milliseconds
}

func NewPoller(svc *gmail.Service, labels []string) *Poller {
	return &Poller{
		svc:         svc,
		labels:      labels,
		lastChecked: time.Now().Add(-24 * time.Hour).UnixMilli(),
	}
}

func (p *Poller) Poll(ctx context.Context) ([]RawEmail, error) {
	query := buildQuery(p.labels, p.lastChecked)
	listRes, err := p.svc.Users.Messages.List("me").Q(query).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	var emails []RawEmail
	for _, m := range listRes.Messages {
		msg, err := p.svc.Users.Messages.Get("me", m.Id).Format("full").Context(ctx).Do()
		if err != nil {
			log.Warnf("gmail: get message %s: %v", m.Id, err)
			continue
		}

		emailType := Classify(msg)
		if emailType == UnknownEmail {
			continue
		}

		raw := RawEmail{
			ID:         msg.Id,
			Subject:    getSubject(msg),
			Type:       emailType,
			ReceivedAt: time.UnixMilli(msg.InternalDate),
		}

		if emailType == AlertEmail {
			raw.Body = extractBody(msg)
		} else {
			raw.PDFText = extractPDFAttachment(msg)
		}

		if raw.Body != "" || raw.PDFText != "" {
			emails = append(emails, raw)
		}
	}

	p.lastChecked = time.Now().UnixMilli()
	return emails, nil
}

func buildQuery(labels []string, afterMs int64) string {
	after := time.UnixMilli(afterMs).Format("2006/01/02")
	q := "after:" + after
	if len(labels) > 0 {
		labelParts := make([]string, len(labels))
		for i, l := range labels {
			labelParts[i] = "label:" + l
		}
		q = "(" + strings.Join(labelParts, " OR ") + ") " + q
	}
	return q
}

func extractBody(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	return extractPartText(msg.Payload)
}

func extractPartText(part *gmail.MessagePart) string {
	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			return ""
		}
		return string(data)
	}
	for _, p := range part.Parts {
		if t := extractPartText(p); t != "" {
			return t
		}
	}
	return ""
}

func extractPDFAttachment(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	for _, part := range msg.Payload.Parts {
		if part.MimeType == "application/pdf" || strings.HasSuffix(strings.ToLower(part.Filename), ".pdf") {
			if part.Body == nil || part.Body.Data == "" {
				continue
			}
			data, err := base64.URLEncoding.DecodeString(part.Body.Data)
			if err != nil {
				continue
			}
			text, err := ExtractPDFText(data)
			if err != nil {
				log.Warnf("gmail: PDF extract failed for %s: %v", part.Filename, err)
				continue
			}
			return text
		}
	}
	return ""
}
