// Package httpapi exposes the service over HTTP and produces the OpenAPI
// document that the dashboard, SDK, and phone app are generated from.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"

	"github.com/simhook/simhook/internal/auth"
	"github.com/simhook/simhook/internal/billing"
	"github.com/simhook/simhook/internal/config"
	"github.com/simhook/simhook/internal/gateway"
	"github.com/simhook/simhook/internal/webhooks"
)

// Deps are the services the handlers call.
type Deps struct {
	Config   *config.Config
	Log      *slog.Logger
	Auth     *auth.Service
	Billing  *billing.Service
	Gateway  *gateway.Service
	Webhooks *webhooks.Service
}

// Server is the HTTP surface.
type Server struct {
	deps    Deps
	api     huma.API
	router  chi.Router
	limiter *ipLimiter
}

// Session cookie name.
const sessionCookie = "simhook_session"

// Security scheme names used in the OpenAPI document.
const (
	secAPIKey  = "apiKey"
	secSession = "session"
	secDevice  = "deviceToken"
)

var (
	securityUser   = []map[string][]string{{secAPIKey: {}}, {secSession: {}}}
	securityDevice = []map[string][]string{{secDevice: {}}}
)

// New builds the router. Deps may be partially nil when only the OpenAPI
// document is needed.
func New(deps Deps) *Server {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	router := chi.NewRouter()
	if deps.Config.TrustProxy {
		// Only behind a proxy that rewrites X-Forwarded-For, such as Caddy in deploy/.
		router.Use(middleware.RealIP)
	}
	router.Use(middleware.RequestID)
	router.Use(recoverer(deps.Log))
	router.Use(requestLogger(deps.Log))
	router.Use(cors(deps.Config))
	router.Use(middleware.Timeout(60 * time.Second))

	cfg := huma.DefaultConfig("simhook API", "1.0.0")
	cfg.Info.Description = "Send and receive SMS through your own Android phone. Authenticate with an API key in the X-Api-Key header."
	cfg.Servers = []*huma.Server{{URL: deps.Config.PublicURL}}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		secAPIKey:  {Type: "apiKey", In: "header", Name: "X-Api-Key", Description: "Developer API key from the dashboard."},
		secSession: {Type: "apiKey", In: "cookie", Name: sessionCookie, Description: "Dashboard session cookie."},
		secDevice:  {Type: "http", Scheme: "bearer", Description: "Device token issued at pairing. Only valid on /v1/device endpoints."},
	}
	cfg.DocsPath = "/docs"
	cfg.OpenAPIPath = "/openapi"
	// The default create hook injects a $schema field and Link header into
	// every response. Dropping it keeps response bodies to exactly the
	// documented shape.
	cfg.CreateHooks = nil

	api := humachi.New(router, cfg)
	s := &Server{deps: deps, api: api, router: router, limiter: newIPLimiter(rate.Every(3*time.Second), 20)}
	api.UseMiddleware(s.authenticate)

	s.registerMisc()
	s.registerAuth()
	s.registerDevices()
	s.registerMessages()
	s.registerWebhooks()
	s.registerPhone()
	return s
}

// Handler returns the http.Handler.
func (s *Server) Handler() http.Handler { return s.router }

// OpenAPI returns the generated document.
func (s *Server) OpenAPI() *huma.OpenAPI { return s.api.OpenAPI() }

// ---------------------------------------------------------------------------
// Authentication middleware
// ---------------------------------------------------------------------------

// credentialPaths are throttled per IP because they accept guesses.
var credentialPaths = map[string]bool{
	"/v1/auth/login":                  true,
	"/v1/auth/register":               true,
	"/v1/auth/password-reset/request": true,
	"/v1/auth/password-reset":         true,
	"/v1/auth/verify-email":           true,
	"/v1/device/pair":                 true,
}

func (s *Server) authenticate(ctx huma.Context, next func(huma.Context)) {
	if ctx.Method() == http.MethodPost && credentialPaths[ctx.URL().Path] {
		if err := s.throttle(ctx); err != nil {
			_ = huma.WriteErr(s.api, ctx, http.StatusTooManyRequests, err.Error())
			return
		}
	}
	ctx = huma.WithValue(ctx, userAgentKey{}, ctx.Header("User-Agent"))
	ctx = huma.WithValue(ctx, remoteAddrKey{}, ctx.RemoteAddr())
	if s.deps.Auth == nil {
		next(ctx)
		return
	}
	var session string
	if c, err := huma.ReadCookie(ctx, sessionCookie); err == nil {
		session = c.Value
		ctx = huma.WithValue(ctx, sessionTokenKey{}, session)
	}
	apiKey := ctx.Header("X-Api-Key")
	bearer := ""
	if h := ctx.Header("Authorization"); strings.HasPrefix(h, "Bearer ") {
		bearer = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	p, err := s.deps.Auth.Authenticate(ctx.Context(), session, apiKey, bearer)
	if err != nil {
		status := http.StatusInternalServerError
		var apiE *APIError
		if e := mapErr(ctx.Context(), s.deps.Log, err); errors.As(e, &apiE) {
			status = apiE.Status
		}
		_ = huma.WriteErr(s.api, ctx, status, mapErr(ctx.Context(), s.deps.Log, err).Error())
		return
	}
	if p != nil {
		ctx = huma.WithValue(ctx, principalKey{}, p)
	}
	next(ctx)
}

type principalKey struct{}

func principal(ctx context.Context) *auth.Principal {
	p, _ := ctx.Value(principalKey{}).(*auth.Principal)
	return p
}

// requireUser returns the account behind a session or API key, checking the
// scope for API keys.
func requireUser(ctx context.Context, scope string) (*auth.Principal, error) {
	p := principal(ctx)
	if p == nil || p.User == nil || p.Kind == auth.KindDevice {
		return nil, apiErr(http.StatusUnauthorized, "unauthenticated", "Sign in or pass an API key in X-Api-Key.")
	}
	if scope != "" && !p.HasScope(scope) {
		return nil, apiErr(http.StatusForbidden, "insufficient_scope", "This API key lacks the "+scope+" scope.")
	}
	return p, nil
}

// requireSession is for account operations that must not be done with a key.
func requireSession(ctx context.Context) (*auth.Principal, error) {
	p := principal(ctx)
	if p == nil || p.Kind != auth.KindSession || p.User == nil {
		return nil, apiErr(http.StatusUnauthorized, "session_required", "Sign in to the dashboard to do this.")
	}
	return p, nil
}

// requireDevice returns the phone behind a device token.
func requireDevice(ctx context.Context) (*auth.Principal, error) {
	p := principal(ctx)
	if p == nil || p.Kind != auth.KindDevice || p.Device == nil {
		return nil, apiErr(http.StatusUnauthorized, "device_token_required", "Pass the device token as a bearer token.")
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Plain middleware
// ---------------------------------------------------------------------------

// cors admits the dashboard and, when configured, the public site: both are
// first-party origins that talk to the API with the session cookie.
func cors(cfg *config.Config) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	if cfg != nil {
		for _, u := range []string{cfg.WebURL, cfg.SiteURL} {
			if u = strings.TrimRight(u, "/"); u != "" {
				allowed[u] = true
			}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if o := r.Header.Get("Origin"); o != "" && allowed[o] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", o)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Max-Age", "600")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if strings.HasPrefix(r.URL.Path, "/healthz") {
				return
			}
			log.Info("http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "ms", time.Since(start).Milliseconds(),
				"ip", r.RemoteAddr, "req", middleware.GetReqID(r.Context()))
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					log.Error("panic", "err", rec, "path", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"status":500,"code":"internal_error","message":"Something went wrong on our side."}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// ipLimiter throttles credential endpoints per client address.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	rate     rate.Limit
	burst    int
}

type entry struct {
	l    *rate.Limiter
	seen time.Time
}

func newIPLimiter(r rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{limiters: map[string]*entry{}, rate: r, burst: burst}
}

func (l *ipLimiter) allow(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.limiters) > 10000 {
		for k, e := range l.limiters {
			if now.Sub(e.seen) > 10*time.Minute {
				delete(l.limiters, k)
			}
		}
	}
	e, ok := l.limiters[host]
	if !ok {
		e = &entry{l: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[host] = e
	}
	e.seen = now
	return e.l.Allow()
}

// throttle rejects when the caller has exhausted the credential budget.
func (s *Server) throttle(ctx huma.Context) error {
	if !s.limiter.allow(ctx.RemoteAddr()) {
		return apiErr(http.StatusTooManyRequests, "rate_limited", "Too many attempts. Wait a minute and try again.")
	}
	return nil
}
