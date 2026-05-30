package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ParsedTransaction struct {
	Date                   string  `json:"date"`
	Amount                 float64 `json:"amount"`
	Currency               string  `json:"currency"`
	Merchant               string  `json:"merchant"`
	AccountLast4           string  `json:"account_last4"`
	Bank                   string  `json:"bank"`
	RefID                  string  `json:"ref_id"`
	TxType                 string  `json:"tx_type"`
	SuggestedLedgerAccount string  `json:"suggested_ledger_account"`
	Confidence             float64 `json:"confidence"`
}

type Parser struct {
	url   string
	model string
}

func New(url, model string) *Parser {
	return &Parser{url: url, model: model}
}

var responseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"date":                     map[string]string{"type": "string"},
		"amount":                   map[string]string{"type": "number"},
		"currency":                 map[string]string{"type": "string"},
		"merchant":                 map[string]string{"type": "string"},
		"account_last4":            map[string]string{"type": "string"},
		"bank":                     map[string]string{"type": "string"},
		"ref_id":                   map[string]string{"type": "string"},
		"tx_type":                  map[string]string{"type": "string"},
		"suggested_ledger_account": map[string]string{"type": "string"},
		"confidence":               map[string]string{"type": "number"},
	},
	"required": []string{"date", "amount", "currency", "merchant", "tx_type", "confidence"},
}

var multiSchema = map[string]interface{}{
	"type":  "array",
	"items": responseSchema,
}

func (p *Parser) Parse(rawText string, knownAccounts []string) (ParsedTransaction, error) {
	accountHint := ""
	if len(knownAccounts) > 0 {
		accountHint = fmt.Sprintf("\nKnown ledger accounts (choose suggested_ledger_account from these): %s",
			strings.Join(knownAccounts, ", "))
	}

	prompt := fmt.Sprintf(`Extract transaction details from this bank notification as JSON.
amount: negative for debits (money leaving account), positive for credits.
confidence: 0.0–1.0 certainty of extraction.
date format: YYYY-MM-DD.%s

Text: %s`, accountHint, rawText)

	content, err := p.callOllama(prompt, responseSchema)
	if err != nil {
		return ParsedTransaction{}, err
	}

	var tx ParsedTransaction
	return tx, json.Unmarshal([]byte(content), &tx)
}

func (p *Parser) ParseMultiple(rawText string, knownAccounts []string) ([]ParsedTransaction, error) {
	accountHint := ""
	if len(knownAccounts) > 0 {
		accountHint = fmt.Sprintf("\nKnown ledger accounts: %s", strings.Join(knownAccounts, ", "))
	}

	prompt := fmt.Sprintf(`Extract ALL transactions from this bank statement as a JSON array.
Each element: date (YYYY-MM-DD), amount (negative=debit, positive=credit), currency, merchant, account_last4, bank, ref_id, tx_type, suggested_ledger_account, confidence.%s

Statement text:
%s`, accountHint, rawText)

	content, err := p.callOllama(prompt, multiSchema)
	if err != nil {
		return nil, err
	}

	var txns []ParsedTransaction
	return txns, json.Unmarshal([]byte(content), &txns)
}

func (p *Parser) callOllama(prompt string, schema interface{}) (string, error) {
	body := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"format": schema,
		"stream": false,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(p.url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama returned %d", resp.StatusCode)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return result.Message.Content, nil
}
