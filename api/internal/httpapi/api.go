// Package httpapi exposes the service over HTTP and produces the OpenAPI
// document that the dashboard, SDK, and phone app are generated from.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
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
	"github.com/simhook/simhook/internal/ratelimit"
	"github.com/simhook/simhook/internal/secrets"
	"github.com/simhook/simhook/internal/turnstile"
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
	// Box seals the Google sign-in flow cookie.
	Box *secrets.Box
	// Google runs Google sign-in; nil means the feature is off.
	Google auth.GoogleExchanger
	// Turnstile checks the sign-in forms' bot tokens; nil means off.
	Turnstile turnstile.Verifier
}

// Server is the HTTP surface.
type Server struct {
	deps    Deps
	api     huma.API
	router  chi.Router
	limiter *ratelimit.Keyed
	// origins are the pages allowed to make cookie-authenticated writes.
	origins map[string]bool
}

// clientIPHeader is the one header the proxy in deploy/ fills with the
// visitor's address. Nothing else is trusted: anything a client can send
// itself (X-Forwarded-For, True-Client-IP) would let it pick its own
// throttle bucket.
const clientIPHeader = "X-Real-IP"

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

// scopeExtension names the OpenAPI extension that says which API key scope
// an operation needs ("session" for dashboard-only operations, "device" for
// the phone's). The generated reference renders it.
const scopeExtension = "x-simhook-scope"

// scopeSession marks operations only a signed-in dashboard session may call.
const scopeSession = "session"

func scoped(scope string) map[string]any {
	return map[string]any{scopeExtension: scope}
}

// New builds the router. Deps may be partially nil when only the OpenAPI
// document is needed.
func New(deps Deps) *Server {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	router := chi.NewRouter()
	if deps.Config.TrustProxy {
		router.Use(clientIP(clientIPHeader))
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
	s := &Server{deps: deps, api: api, router: router, limiter: ratelimit.NewKeyed(rate.Every(3*time.Second), 20), origins: map[string]bool{}}
	for _, o := range deps.Config.BrowserOrigins() {
		s.origins[o] = true
	}
	api.UseMiddleware(s.authenticate)

	s.registerMisc()
	s.registerAuth()
	s.registerGoogle()
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

// cookiePaths are the operations that write the cookies themselves; the
// middleware leaves the response's cookies to them.
var cookiePaths = map[string]bool{
	"/v1/auth/login":           true,
	"/v1/auth/register":        true,
	"/v1/auth/logout":          true,
	"/v1/auth/google/callback": true,
}

// authenticate resolves the caller and keeps the browser's cookies truthful.
//
// A session cookie that names no live session is not an error here: the
// caller is simply not signed in, the handler decides what that means, and
// both cookies are cleared so the site and the dashboard stop believing
// otherwise. A live session missing its flag gets the flag back; a session
// whose expiry moved gets both cookies re-issued. Cookie-authenticated
// writes are checked against the allowed origins before anything else.
func (s *Server) authenticate(ctx huma.Context, next func(huma.Context)) {
	path := ctx.URL().Path
	if ctx.Method() == http.MethodPost && credentialPaths[path] {
		if err := s.throttle(ctx); err != nil {
			s.writeError(ctx, err)
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
	if c, err := huma.ReadCookie(ctx, sessionCookie); err == nil && c.Value != "" {
		session = c.Value
		ctx = huma.WithValue(ctx, sessionTokenKey{}, session)
	}
	_, flagErr := huma.ReadCookie(ctx, signedInCookie)
	hasFlag := flagErr == nil
	apiKey := ctx.Header("X-Api-Key")
	bearer := ""
	if h := ctx.Header("Authorization"); strings.HasPrefix(h, "Bearer ") {
		bearer = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	// A browser request is one that carries no credential a browser would
	// not add on its own; only those are subject to cookie rules.
	browser := apiKey == "" && bearer == ""
	if browser && unsafeMethod(ctx.Method()) && !originAllowed(s.origins, ctx.Header("Origin"), ctx.Header("Referer"), ctx.Header("Sec-Fetch-Site")) {
		s.writeError(ctx, errCSRF)
		return
	}

	p, err := s.deps.Auth.Authenticate(ctx.Context(), session, apiKey, bearer)
	if err != nil {
		if browser && session != "" && !cookiePaths[path] {
			setCookies(ctx, s.clearCookies())
		}
		s.writeError(ctx, mapErr(ctx.Context(), s.deps.Log, err))
		return
	}
	if browser && !cookiePaths[path] {
		switch {
		case p == nil && (session != "" || hasFlag):
			setCookies(ctx, s.clearCookies())
		case p != nil && p.Kind == auth.KindSession && p.SessionRefreshed:
			setCookies(ctx, s.issueCookies(session, p.Session.ExpiresAt))
		case p != nil && p.Kind == auth.KindSession && !hasFlag:
			setCookies(ctx, []http.Cookie{s.signedInCookieFor(p.Session.ExpiresAt)})
		}
	}
	if p != nil {
		ctx = huma.WithValue(ctx, principalKey{}, p)
	}
	next(ctx)
}

// writeError writes an error from middleware. huma's own helper would
// rebuild the error from its status and lose the code.
func (s *Server) writeError(ctx huma.Context, err error) {
	var apiE *APIError
	if !errors.As(err, &apiE) {
		apiE = apiErr(http.StatusInternalServerError, "internal_error", "Something went wrong on our side.")
	}
	body, _ := json.Marshal(apiE)
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(apiE.Status)
	_, _ = ctx.BodyWriter().Write(body)
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

// clientIP replaces the connection address with the one the proxy put in
// header, when it is a valid address. Only enabled behind our own proxy.
func clientIP(header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if raw := strings.TrimSpace(r.Header.Get(header)); raw != "" {
				if addr, err := netip.ParseAddr(raw); err == nil {
					r.RemoteAddr = addr.String()
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
			h := w.Header()
			// The answer depends on the origin whether or not this one is
			// allowed, so caches must key on it either way.
			h.Add("Vary", "Origin")
			if o := r.Header.Get("Origin"); o != "" && allowed[o] {
				h.Set("Access-Control-Allow-Origin", o)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Max-Age", "600")
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

// hostOf strips the port from a connection address.
func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// throttle rejects when the caller has exhausted the credential budget.
func (s *Server) throttle(ctx huma.Context) error {
	if !s.limiter.Allow(hostOf(ctx.RemoteAddr())) {
		return apiErr(http.StatusTooManyRequests, "rate_limited", "Too many attempts. Wait a minute and try again.")
	}
	return nil
}
