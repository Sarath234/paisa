package service

import (
	"path/filepath"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
)

func TestLoadVAPIDKeys_CreatesKeysOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	pub, priv, err := LoadVAPIDKeys(dir)
	assert.NoError(t, err)
	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, priv)

	// Keys file must be created
	path := filepath.Join(dir, "vapid_keys.json")
	assert.FileExists(t, path)
}

func TestLoadVAPIDKeys_ReturnsExistingKeys(t *testing.T) {
	dir := t.TempDir()
	// First call creates keys
	pub1, priv1, err := LoadVAPIDKeys(dir)
	assert.NoError(t, err)

	// Second call must return same keys
	pub2, priv2, err := LoadVAPIDKeys(dir)
	assert.NoError(t, err)
	assert.Equal(t, pub1, pub2)
	assert.Equal(t, priv1, priv2)
}

func TestSaveAndLoadSubscriptions(t *testing.T) {
	dir := t.TempDir()
	sub := &webpush.Subscription{
		Endpoint: "https://push.example.com/abc123",
		Keys: webpush.Keys{
			Auth:   "auth-key",
			P256dh: "p256dh-key",
		},
	}

	err := SaveSubscription(dir, sub)
	assert.NoError(t, err)

	subs, err := LoadSubscriptions(dir)
	assert.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, sub.Endpoint, subs[0].Endpoint)
}

func TestSaveSubscriptions_DeduplicatesByEndpoint(t *testing.T) {
	dir := t.TempDir()
	sub := &webpush.Subscription{
		Endpoint: "https://push.example.com/abc123",
		Keys:     webpush.Keys{Auth: "a", P256dh: "b"},
	}

	_ = SaveSubscription(dir, sub)
	_ = SaveSubscription(dir, sub) // duplicate

	subs, err := LoadSubscriptions(dir)
	assert.NoError(t, err)
	assert.Len(t, subs, 1)
}

