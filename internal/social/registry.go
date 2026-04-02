package social

import "github.com/simplyblock/postbrain/internal/config"

// NewRegistry builds enabled social providers from OAuth config.
func NewRegistry(cfg config.OAuthConfig) map[string]Provider {
	out := make(map[string]Provider)
	for name, pcfg := range cfg.Providers {
		if !pcfg.Enabled {
			continue
		}
		switch name {
		case "github":
			out[name] = NewGitHubProvider(cfg.BaseURL, pcfg)
		case "google":
			out[name] = NewGoogleProvider(cfg.BaseURL, pcfg)
		case "gitlab":
			out[name] = NewGitLabProvider(cfg.BaseURL, pcfg)
		}
	}
	return out
}
