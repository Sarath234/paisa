// internal/agent/telegram/bot.go
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Bot wraps the Telegram Bot API using raw HTTP (no external dependency).
type Bot struct {
	Token  string
	ChatID int64
	offset int
	client *http.Client
}

func NewBot(token string, chatID int64) *Bot {
	return &Bot{
		Token:  token,
		ChatID: chatID,
		client: &http.Client{Timeout: 40 * time.Second},
	}
}

// Telegram API types

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type apiResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

func (b *Bot) apiURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.Token, method)
}

func (b *Bot) post(method string, payload any) (json.RawMessage, error) {
	body, _ := json.Marshal(payload)
	resp, err := b.client.Post(b.apiURL(method), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var ar apiResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return nil, fmt.Errorf("%s parse response: %w", method, err)
	}
	if !ar.OK {
		return nil, fmt.Errorf("%s api error: %s", method, string(data))
	}
	return ar.Result, nil
}

// Poll fetches pending updates via long-polling (30s timeout). Advances offset.
func (b *Bot) Poll() ([]Update, error) {
	result, err := b.post("getUpdates", map[string]any{
		"offset":  b.offset,
		"timeout": 30,
	})
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(result, &updates); err != nil {
		return nil, err
	}
	for _, u := range updates {
		if u.UpdateID >= b.offset {
			b.offset = u.UpdateID + 1
		}
	}
	return updates, nil
}

// draftButtons is the approval draft's inline keyboard: ✅ Approve / ✏️ Edit / ⏭ Skip
// on the first row, with 🍔 Food / 🛒 Groceries quick-category shortcuts on the second.
func draftButtons() [][]inlineButton {
	return [][]inlineButton{
		{
			{Text: "✅ Approve", CallbackData: "approve"},
			{Text: "✏️ Edit", CallbackData: "edit"},
			{Text: "⏭ Skip", CallbackData: "skip"},
		},
		{
			{Text: "🍔 Food", CallbackData: "food"},
			{Text: "🛒 Groceries", CallbackData: "groceries"},
		},
	}
}

// SendDraft sends the approval draft with ✅ Approve / ✏️ Edit / ⏭ Skip buttons,
// plus 🍔 Food / 🛒 Groceries shortcuts to quickly set dest.
// Returns the sent message's ID for later editing.
func (b *Bot) SendDraft(text string) (int, error) {
	return b.sendWithKeyboard(text, draftButtons())
}

// SendDraftDuplicate sends the duplicate-warning draft with ✅ Post anyway / ⏭ Skip.
func (b *Bot) SendDraftDuplicate(text string) (int, error) {
	warning := "⚠️ Possible duplicate — matching entry exists. Post anyway?\n\n" + text
	return b.sendWithKeyboard(warning, [][]inlineButton{{
		{Text: "✅ Post anyway", CallbackData: "approve"},
		{Text: "⏭ Skip", CallbackData: "skip"},
	}})
}

// SendRuleConfirmation sends the initial rule-learning prompt with 3 buttons.
// Returns the sent message's ID for later editing.
func (b *Bot) SendRuleConfirmation(text string) (int, error) {
	return b.sendWithKeyboard(text, [][]inlineButton{{
		{Text: "✅ Add rule", CallbackData: "add_rule"},
		{Text: "✏️ Edit rule", CallbackData: "edit_rule"},
		{Text: "⏭ Skip", CallbackData: "skip_rule"},
	}})
}

// SendRuleConfirmationFinal sends the post-edit confirmation with 2 buttons (no further edit).
// Returns the sent message's ID for later editing.
func (b *Bot) SendRuleConfirmationFinal(text string) (int, error) {
	return b.sendWithKeyboard(text, [][]inlineButton{{
		{Text: "✅ Add rule", CallbackData: "add_rule"},
		{Text: "⏭ Skip", CallbackData: "skip_rule"},
	}})
}

// SendConfirm sends a duplicate-removal confirmation card with
// ✅ Remove / ❌ Keep buttons. Returns the sent message's ID for later
// editing; the callback handler identifies the pending removal by that
// message ID (same convention as SendDraft/SendRuleConfirmation), so the
// callback data itself stays static.
func (b *Bot) SendConfirm(text string) (int, error) {
	return b.sendWithKeyboard(text, [][]inlineButton{{
		{Text: "✅ Remove", CallbackData: "ccdel"},
		{Text: "❌ Keep", CallbackData: "cckeep"},
	}})
}

func (b *Bot) sendWithKeyboard(text string, buttons [][]inlineButton) (int, error) {
	result, err := b.post("sendMessage", map[string]any{
		"chat_id":      b.ChatID,
		"text":         text,
		"reply_markup": inlineKeyboard{InlineKeyboard: buttons},
	})
	if err != nil {
		return 0, err
	}
	var msg Message
	if err := json.Unmarshal(result, &msg); err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

// SendText sends a plain text message (used for edit template and error messages).
func (b *Bot) SendText(text string) error {
	_, err := b.post("sendMessage", map[string]any{
		"chat_id": b.ChatID,
		"text":    text,
	})
	return err
}

// EditMessage replaces the text of an existing message (removes inline keyboard).
func (b *Bot) EditMessage(messageID int, text string) error {
	_, err := b.post("editMessageText", map[string]any{
		"chat_id":    b.ChatID,
		"message_id": messageID,
		"text":       text,
	})
	return err
}

// EditDraft replaces the text of a pending draft message while keeping its
// Approve/Edit/Skip/Food/Groceries keyboard active, so the user can still act
// on it after a Food/Groceries dest shortcut is tapped.
func (b *Bot) EditDraft(messageID int, text string) error {
	_, err := b.post("editMessageText", map[string]any{
		"chat_id":      b.ChatID,
		"message_id":   messageID,
		"text":         text,
		"reply_markup": inlineKeyboard{InlineKeyboard: draftButtons()},
	})
	return err
}

// AnswerCallback acknowledges a callback query (removes the loading spinner on the button).
func (b *Bot) AnswerCallback(callbackID string) error {
	_, err := b.post("answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
	})
	return err
}
