package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ananthakumaran/paisa/internal/agent/parser"
)

type Chat struct {
	ID int64 `json:"id"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	Data    string  `json:"data"`
	Message Message `json:"message"`
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Bot struct {
	token   string
	chatID  int64
	offset  int
	baseURL string
}

func New(token string, chatID int64) *Bot {
	return &Bot{token: token, chatID: chatID, baseURL: "https://api.telegram.org"}
}

func newWithBaseURL(token string, chatID int64, baseURL string) *Bot {
	return &Bot{token: token, chatID: chatID, baseURL: baseURL}
}

func (b *Bot) GetUpdates() ([]Update, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=30", b.baseURL, b.token, b.offset)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Result []Update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	for _, u := range result.Result {
		if u.UpdateID >= b.offset {
			b.offset = u.UpdateID + 1
		}
	}
	return result.Result, nil
}

func (b *Bot) SendApprovalCard(tx parser.ParsedTransaction) error {
	absAmount := tx.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}
	text := fmt.Sprintf("*%s*  ·  %s XX%s\n%.2f %s  ·  %s\n→ _%s_ (suggested)",
		tx.Merchant, tx.Bank, tx.AccountLast4,
		absAmount, tx.Currency, tx.Date,
		tx.SuggestedLedgerAccount)

	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{{
			{"text": "✓ Post", "callback_data": "approve:" + tx.RefID},
			{"text": "✎ Edit", "callback_data": "edit:" + tx.RefID},
			{"text": "✗ Skip", "callback_data": "skip:" + tx.RefID},
		}},
	}
	return b.sendMessage(text, keyboard)
}

func (b *Bot) SendText(text string) error {
	return b.sendMessage(text, nil)
}

func (b *Bot) AnswerCallback(callbackID string) error {
	body := map[string]string{"callback_query_id": callbackID}
	data, _ := json.Marshal(body)
	resp, err := http.Post(
		fmt.Sprintf("%s/bot%s/answerCallbackQuery", b.baseURL, b.token),
		"application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *Bot) sendMessage(text string, keyboard interface{}) error {
	body := map[string]interface{}{
		"chat_id":    b.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	if keyboard != nil {
		body["reply_markup"] = keyboard
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(
		fmt.Sprintf("%s/bot%s/sendMessage", b.baseURL, b.token),
		"application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}
