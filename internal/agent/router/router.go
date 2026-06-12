// internal/agent/router/router.go
// Package router dispatches incoming Telegram messages to capabilities.
// Deterministic matchers run first (fast path); only unmatched text pays for
// an LLM intent-classification call. Matchers may only claim machine-generated
// or explicitly structured text (bank SMS, "/" commands) — natural language
// always falls through to the LLM stage.
package router

import (
	log "github.com/sirupsen/logrus"
)

// Capability is a pluggable message handler. Implementations: sms.Capability,
// qa.Capability.
type Capability interface {
	Name() string
	// Match reports whether this capability deterministically claims the
	// message (fast path). High-precision only.
	Match(text string) bool
	// Handle processes a message routed to this capability.
	Handle(text string) error
	// HasPending reports whether this capability has an open multi-turn
	// conversation awaiting the next message from this chat.
	HasPending(chatID int64) bool
	// HandleReply consumes the next message of an open conversation.
	HandleReply(text string) error
}

type Router struct {
	caps     []Capability
	classify func(text string) (string, error)
	fallback func(text string)
}

// New builds a router. classify maps text → capability name (or "unknown");
// fallback runs when nothing claims the message.
func New(caps []Capability, classify func(string) (string, error), fallback func(string)) *Router {
	return &Router{caps: caps, classify: classify, fallback: fallback}
}

// Route dispatches one message: open conversations first, then deterministic
// matchers in registration order, then LLM classification, then fallback.
func (r *Router) Route(chatID int64, text string) {
	for _, c := range r.caps {
		if c.HasPending(chatID) {
			log.Debugf("router: %s has pending conversation — routing reply", c.Name())
			if err := c.HandleReply(text); err != nil {
				log.Errorf("router: %s reply: %v", c.Name(), err)
			}
			return
		}
	}

	for _, c := range r.caps {
		if c.Match(text) {
			log.Infof("router: fast path → %s", c.Name())
			if err := c.Handle(text); err != nil {
				log.Errorf("router: %s handle: %v", c.Name(), err)
			}
			return
		}
	}

	intent, err := r.classify(text)
	if err != nil {
		log.Warnf("router: classify failed: %v — falling back", err)
		r.fallback(text)
		return
	}
	for _, c := range r.caps {
		if c.Name() == intent {
			log.Infof("router: llm → %s", c.Name())
			if err := c.Handle(text); err != nil {
				log.Errorf("router: %s handle: %v", c.Name(), err)
			}
			return
		}
	}
	log.Infof("router: intent %q unhandled — falling back", intent)
	r.fallback(text)
}
