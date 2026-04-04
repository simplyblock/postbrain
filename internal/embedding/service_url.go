package embedding

import (
	"strings"

	"github.com/simplyblock/postbrain/internal/config"
)

const defaultOllamaServiceURL = "http://localhost:11434"

func serviceURLOrDefault(cfg *config.EmbeddingConfig, fallback string) string {
	if cfg == nil {
		return fallback
	}
	if u := strings.TrimSpace(cfg.ServiceURL); u != "" {
		return u
	}
	return fallback
}
