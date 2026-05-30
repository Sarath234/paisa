package main

import (
	"context"
	"flag"
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
						rawText := email.Body
						source := "gmail_alert"
						if email.Type == gmail.StatementEmail {
							rawText = email.PDFText
							source = "gmail_statement"
						}
						if rawText == "" {
							continue
						}
						if _, err := pipe.Process(rawText, source); err != nil {
							log.Warnf("pipeline gmail: %v", err)
						}
					}
				}
				time.Sleep(interval)
			}
		}()
	}

	log.Infof("paisa-agent running, paisa URL: %s", cfg.Paisa.URL)
	select {}
}
