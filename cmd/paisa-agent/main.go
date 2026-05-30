package main

import (
	"flag"
	"os"
	"path/filepath"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "", "path to paisa-agent.yaml (default: ~/.config/paisa-agent/paisa-agent.yaml)")
	flag.Parse()

	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		cfgPath = filepath.Join(home, ".config", "paisa-agent", "paisa-agent.yaml")
	}

	cfg, err := agentconfig.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Infof("paisa-agent starting, paisa URL: %s", cfg.Paisa.URL)
	// goroutines added in Task 11
	select {}
}
