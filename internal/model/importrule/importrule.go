package importrule

import (
	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

func All() []config.ImportRule {
	return config.GetConfig().ImportRules
}

func Upsert(rule config.ImportRule) config.ImportRule {
	Delete(rule.Name)
	cfg := config.GetConfig()
	cfg.ImportRules = append(cfg.ImportRules, rule)
	if err := config.SaveConfigObject(cfg); err != nil {
		log.Fatal(err)
	}
	return rule
}

func Delete(name string) {
	cfg := config.GetConfig()
	cfg.ImportRules = lo.Filter(cfg.ImportRules, func(r config.ImportRule, _ int) bool {
		return r.Name != name
	})
	if err := config.SaveConfigObject(cfg); err != nil {
		log.Fatal(err)
	}
}
