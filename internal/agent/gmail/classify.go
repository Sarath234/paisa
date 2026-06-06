package gmail

import (
	"strings"

	"google.golang.org/api/gmail/v1"
)

type EmailType int

const (
	UnknownEmail   EmailType = iota
	AlertEmail               // transaction notification email, parse body
	StatementEmail           // monthly statement with PDF attachment
)

var alertKeywords = []string{
	"debited", "credited", "transaction alert", "payment", "spent",
	"rs.", "inr", "amount", "debit", "credit",
}

var statementKeywords = []string{"statement", "e-statement", "account summary", "monthly statement"}

func Classify(msg *gmail.Message) EmailType {
	subject := strings.ToLower(getSubject(msg))

	hasPDF := false
	if msg.Payload != nil {
		for _, part := range msg.Payload.Parts {
			if part.MimeType == "application/pdf" || strings.HasSuffix(strings.ToLower(part.Filename), ".pdf") {
				hasPDF = true
				break
			}
		}
	}

	for _, kw := range statementKeywords {
		if strings.Contains(subject, kw) && hasPDF {
			return StatementEmail
		}
	}

	for _, kw := range alertKeywords {
		if strings.Contains(subject, kw) {
			return AlertEmail
		}
	}

	return UnknownEmail
}

func getSubject(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}
	for _, h := range msg.Payload.Headers {
		if h.Name == "Subject" {
			return h.Value
		}
	}
	return ""
}
