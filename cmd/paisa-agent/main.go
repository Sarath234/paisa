package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	agentdb "github.com/ananthakumaran/paisa/internal/agent/db"
	"github.com/ananthakumaran/paisa/internal/agent/gmail"
	"github.com/ananthakumaran/paisa/internal/agent/merchant"
	"github.com/ananthakumaran/paisa/internal/agent/pipeline"
	log "github.com/sirupsen/logrus"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "path to paisa-agent.yaml")
	flag.Parse()

	if cfgPath == "" {
		home, _ := os.UserHomeDir()
		cfgPath = filepath.Join(home, ".config", "paisa-agent", "paisa-agent.yaml")
	}

	cfg, err := agentconfig.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbPath := filepath.Join(os.Getenv("HOME"), ".local", "share", "paisa-agent", "agent.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := agentdb.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	if cfg.Paisa.JournalDir != "" {
		journalPath := filepath.Join(cfg.Paisa.JournalDir, "main.journal")
		if err := merchant.Bootstrap(db, journalPath, cfg.MerchantRules.PromoteAfterApprovals); err != nil {
			log.Warnf("bootstrap merchant rules: %v", err)
		}
	}

	pipe := pipeline.New(db, cfg)

	// Goroutine 1: Telegram long-poll (SMS relay + callback handling)
	go func() {
		log.Info("telegram poller started")
		for {
			updates, err := pipe.Bot().GetUpdates()
			if err != nil {
				log.Warnf("telegram getUpdates: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			for _, u := range updates {
				if u.CallbackQuery != nil {
					pipe.HandleCallback(u.CallbackQuery.ID, u.CallbackQuery.Data)
				} else if u.Message != nil && u.Message.Text != "" {
					action, err := pipe.Process(u.Message.Text, "sms")
					if err != nil {
						log.Warnf("pipeline sms: %v", err)
					}
					log.Infof("telegram msg processed: action=%d", action)
				}
			}
		}
	}()

	// Goroutine 2: Gmail poller (skipped if credentials not configured)
	if cfg.Gmail.CredentialsFile != "" {
		go func() {
			log.Info("gmail poller started")
			tokenFile := filepath.Join(filepath.Dir(cfgPath), "gmail-token.json")
			svc, err := gmail.NewService(context.Background(), cfg.Gmail.CredentialsFile, tokenFile)
			if err != nil {
				log.Fatalf("gmail auth: %v", err)
			}
			poller := gmail.NewPoller(svc, cfg.Gmail.Labels)
			interval := time.Duration(cfg.Gmail.PollIntervalSeconds) * time.Second

			for {
				emails, err := poller.Poll(context.Background())
				if err != nil {
					log.Warnf("gmail poll: %v", err)
				} else {
					for _, email := range emails {
						if email.Type == gmail.StatementEmail {
							gaps, dups, err := pipe.ProcessStatement(email.PDFText)
							if err != nil {
								log.Warnf("pipeline statement: %v", err)
							} else {
								log.Infof("statement processed: %d gaps, %d dups", gaps, dups)
							}
						} else {
							if email.Body == "" {
								continue
							}
							if _, err := pipe.Process(email.Body, "gmail_alert"); err != nil {
								log.Warnf("pipeline gmail: %v", err)
							}
						}
					}
				}
				time.Sleep(interval)
			}
		}()
	}

	// HTTP server for UI parse requests
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/parse", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
				http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
				return
			}
			tx, err := pipe.ParseText(req.Text)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tx)
		})
		addr := cfg.Listen
		if addr == "" {
			addr = "127.0.0.1:7501"
		}
		log.Infof("paisa-agent HTTP listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Errorf("paisa-agent HTTP server: %v", err)
		}
	}()

	log.Infof("paisa-agent running, paisa URL: %s", cfg.Paisa.URL)
	select {}
}
