// Package ui provides the HTTP handler for the Postbrain Web UI.
package ui

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/social"
)

//go:embed web/templates
var templatesFS embed.FS

//go:embed web/static
var staticFS embed.FS

// Handler holds all dependencies for the UI.
type Handler struct {
	pool       *pgxpool.Pool
	templates  *template.Template
	staticFS   fs.FS
	knwLife    *knowledge.Lifecycle
	knwStore   *knowledge.Store
	knwProm    *knowledge.Promoter
	memStore   *memory.Store
	svc        *providers.EmbeddingService
	syncer     *codegraph.Syncer
	oauthCfg   config.OAuthConfig
	providers  map[string]social.Provider
	stateStore oauthStateStore
	codeStore  oauthCodeStore
	clients    oauthClientLookup
	issuer     oauthIssuer
	identities socialIdentityStore
}

// NewHandler creates a UI Handler with parsed templates.
func NewHandler(pool *pgxpool.Pool, svc *providers.EmbeddingService, cfg *config.Config) (*Handler, error) {
	funcMap := template.FuncMap{
		"truncate":    truncate,
		"timeAgo":     timeAgo,
		"deref":       derefString,
		"derefTime":   derefTime,
		"statusBadge": statusBadge,
		"join":        strings.Join,
		"add1":        func(i int) int { return i + 1 },
		"tokenHasScope": func(tok *db.Token, scopeID uuid.UUID) bool {
			if tok == nil {
				return false
			}
			for _, id := range tok.ScopeIds {
				if id == scopeID {
					return true
				}
			}
			return false
		},
		"slice": func(s string, i, j int) string {
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "web/templates/*.html")
	if err != nil {
		return nil, err
	}
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		return nil, err
	}
	h := &Handler{pool: pool, templates: tmpl, staticFS: sub, syncer: codegraph.NewSyncer(cfg.CodeGraph), providers: map[string]social.Provider{}}
	if pool != nil {
		membership := principals.NewMembershipStore(pool)
		h.knwLife = knowledge.NewLifecycle(pool, membership)
		h.knwProm = knowledge.NewPromoter(pool, svc)
	}
	if pool != nil && svc != nil {
		h.knwStore = knowledge.NewStore(pool, svc)
		h.memStore = memory.NewStore(pool, svc)
		h.svc = svc
	}
	return h, nil
}

// NewHandlerWithOAuth creates a UI handler with OAuth/social dependencies wired.
func NewHandlerWithOAuth(
	pool *pgxpool.Pool,
	svc *providers.EmbeddingService,
	cfg *config.Config,
	providers map[string]social.Provider,
	stateStore *oauth.StateStore,
	clientStore *oauth.ClientStore,
	codeStore *oauth.CodeStore,
	issuer *oauth.Issuer,
	identities *social.IdentityStore,
) (*Handler, error) {
	h, err := NewHandler(pool, svc, cfg)
	if err != nil {
		return nil, err
	}
	h.oauthCfg = cfg.OAuth
	h.providers = providers
	h.stateStore = stateStore
	h.clients = clientStore
	h.codeStore = codeStore
	h.issuer = issuer
	h.identities = identities
	return h, nil
}
