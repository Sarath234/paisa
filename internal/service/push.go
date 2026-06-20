package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/SherClockHolmes/webpush-go"
	log "github.com/sirupsen/logrus"
)

var pushMu sync.Mutex

type vapidKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

type pushStore struct {
	Subscriptions []webpush.Subscription `json:"subscriptions"`
}

// LoadVAPIDKeys loads existing VAPID keys from dir, or generates and saves new ones.
func LoadVAPIDKeys(dir string) (publicKey, privateKey string, err error) {
	path := filepath.Join(dir, "vapid_keys.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var keys vapidKeys
		if jsonErr := json.Unmarshal(data, &keys); jsonErr == nil {
			return keys.Public, keys.Private, nil
		}
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}

	keys := vapidKeys{Public: pub, Private: priv}
	out, _ := json.Marshal(keys)
	if writeErr := os.WriteFile(path, out, 0600); writeErr != nil {
		log.Warnf("push: could not save VAPID keys: %v", writeErr)
	}
	return pub, priv, nil
}

// SaveSubscription saves a push subscription, deduplicating by endpoint.
func SaveSubscription(dir string, sub *webpush.Subscription) error {
	pushMu.Lock()
	defer pushMu.Unlock()

	subs, err := LoadSubscriptions(dir)
	if err != nil {
		subs = []webpush.Subscription{}
	}

	filtered := subs[:0]
	for _, s := range subs {
		if s.Endpoint != sub.Endpoint {
			filtered = append(filtered, s)
		}
	}
	filtered = append(filtered, *sub)

	store := pushStore{Subscriptions: filtered}
	data, _ := json.Marshal(store)
	return os.WriteFile(filepath.Join(dir, "push_subscriptions.json"), data, 0600)
}

// LoadSubscriptions reads all stored push subscriptions from dir.
func LoadSubscriptions(dir string) ([]webpush.Subscription, error) {
	path := filepath.Join(dir, "push_subscriptions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []webpush.Subscription{}, nil
		}
		return nil, err
	}
	var store pushStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store.Subscriptions, nil
}

// SendPushNotification sends a notification to all stored subscriptions,
// removing expired ones (404/410 responses).
func SendPushNotification(dir, publicKey, privateKey, title, body string) {
	pushMu.Lock()
	subs, err := LoadSubscriptions(dir)
	pushMu.Unlock()
	if err != nil || len(subs) == 0 {
		return
	}

	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	var active []webpush.Subscription

	for _, sub := range subs {
		s := sub
		resp, err := webpush.SendNotification(payload, &s, &webpush.Options{
			Subscriber:      "mailto:paisa@paisa.app",
			VAPIDPublicKey:  publicKey,
			VAPIDPrivateKey: privateKey,
			TTL:             86400,
		})
		if err != nil {
			log.Warnf("push: send error for %s: %v", sub.Endpoint, err)
			active = append(active, sub)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 404 || resp.StatusCode == 410 {
			log.Infof("push: removing expired subscription %s", sub.Endpoint)
		} else {
			active = append(active, sub)
		}
	}

	pushMu.Lock()
	store := pushStore{Subscriptions: active}
	data, _ := json.Marshal(store)
	_ = os.WriteFile(filepath.Join(dir, "push_subscriptions.json"), data, 0600)
	pushMu.Unlock()
}
