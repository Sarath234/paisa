package server

import (
	"net/http"
	"os"
	"path/filepath"

	agentconfig "github.com/ananthakumaran/paisa/internal/agent/config"
	"github.com/ananthakumaran/paisa/internal/agent/parser"
	"github.com/gin-gonic/gin"
)

func ParseSMS(c *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	cfgPath := filepath.Join(os.Getenv("HOME"), ".config", "paisa-agent", "paisa-agent.yaml")
	cfg, err := agentconfig.Load(cfgPath)
	if err != nil {
		cfg = agentconfig.DefaultConfig()
	}

	accounts := make([]string, 0, len(cfg.ParserRules.Sources))
	for _, s := range cfg.ParserRules.Sources {
		accounts = append(accounts, s.Account)
	}

	p := parser.New(cfg.Ollama.URL, cfg.Ollama.Model, cfg.ParserRules)
	tx, err := p.Parse(req.Text, accounts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tx)
}
