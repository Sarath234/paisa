package gmail

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/gmail/v1"
)

func TestClassify_AlertEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "HDFC Bank: Rs.2450 debited from your account"},
			},
		},
	}
	assert.Equal(t, AlertEmail, Classify(msg))
}

func TestClassify_StatementEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Your HDFC Bank e-Statement for April 2025"},
			},
			Parts: []*gmail.MessagePart{
				{MimeType: "application/pdf", Filename: "statement.pdf"},
			},
		},
	}
	assert.Equal(t, StatementEmail, Classify(msg))
}

func TestClassify_UnrelatedEmail(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Your order has shipped"},
			},
		},
	}
	assert.Equal(t, UnknownEmail, Classify(msg))
}

func TestGetSubject(t *testing.T) {
	msg := &gmail.Message{
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Subject", Value: "Test Subject"},
				{Name: "From", Value: "bank@hdfc.com"},
			},
		},
	}
	assert.Equal(t, "Test Subject", getSubject(msg))
}
