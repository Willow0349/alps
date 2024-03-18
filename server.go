package alps

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fernet/fernet-go"
	"github.com/labstack/echo/v4"
)

const (
	cookieName           = "alps_session"
	loginTokenCookieName = "alps_login_token"
)

// Server holds all the alps server state.
type Server struct {
	e        *echo.Echo
	Sessions *SessionManager
	Options  *Options

	mutex   sync.RWMutex // used for server reload
	plugins []Plugin

	// maps protocols to URLs (protocol can be empty for auto-discovery)
	upstreams map[string]*url.URL

	imap struct {
		host     string
		tls      bool
		insecure bool
	}
	smtp struct {
		host     string
		tls      bool
		insecure bool
	}
}

func newServer(e *echo.Echo, options *Options) (*Server, error) {
	s := &Server{e: e, Options: options}

	s.upstreams = make(map[string]*url.URL, len(options.Upstreams))
	for _, upstream := range options.Upstreams {
		u, err := parseUpstream(upstream)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upstream %q: %v", upstream, err)
		}
		if _, ok := s.upstreams[u.Scheme]; ok {
			return nil, fmt.Errorf("found two upstream servers for scheme %q", u.Scheme)
		}
		s.upstreams[u.Scheme] = u
	}

	if err := s.parseIMAPUpstream(); err != nil {
		return nil, err
	}
	if err := s.parseSMTPUpstream(); err != nil {
		return nil, err
	}

	s.Sessions = newSessionManager(s.dialIMAP, s.dialSMTP, e.Logger)
	return s, nil
}

func (s *Server) Close() {
	s.Sessions.Close()
}

func parseUpstream(s string) (*url.URL, error) {
	if !strings.ContainsAny(s, ":/") {
		// This is a raw domain name, make it an URL with an empty scheme
		s = "//" + s
	}
	return url.Parse(s)
}

type NoUpstreamError struct {
	schemes []string
}

func (err *NoUpstreamError) Error() string {
	return fmt.Sprintf("no upstream server configured for schemes %v", err.schemes)
}

// Upstream retrieves the configured upstream server URL for the provided
// schemes. If no configured upstream server matches, a *NoUpstreamError is
// returned. An empty URL.Scheme means that the caller needs to perform
// auto-discovery with URL.Host.
func (s *Server) Upstream(schemes ...string) (*url.URL, error) {
	var urls []*url.URL
	for _, scheme := range append(schemes, "") {
		u, ok := s.upstreams[scheme]
		if ok {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return nil, &NoUpstreamError{schemes}
	}
	if len(urls) > 1 {
		return nil, fmt.Errorf("multiple upstream servers are configured for schemes %v", schemes)
	}
	return urls[0], nil
}

func (s *Server) parseIMAPUpstream() error {
	u, err := s.Upstream("imap", "imaps", "imap+insecure")
	if err != nil {
		return fmt.Errorf("failed to parse upstream IMAP server: %v", err)
	}

	if u.Scheme == "" {
		u, err = discoverIMAP(u.Host)
		if err != nil {
			return fmt.Errorf("failed to discover IMAP server: %v", err)
		}
	}

	switch u.Scheme {
	case "imaps":
		s.imap.tls = true
	case "imap+insecure":
		s.imap.insecure = true
	}

	s.imap.host = u.Host
	if !strings.ContainsRune(s.imap.host, ':') {
		if u.Scheme == "imaps" {
			s.imap.host += ":993"
		} else {
			s.imap.host += ":143"
		}
	}

	c, err := s.dialIMAP()
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %v", err)
	}
	c.Close()

	s.e.Logger.Printf("Configured upstream IMAP server: %v", u)
	return nil
}

func (s *Server) parseSMTPUpstream() error {
	u, err := s.Upstream("smtp", "smtps", "smtp+insecure")
	if _, ok := err.(*NoUpstreamError); ok {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to parse upstream SMTP server: %v", err)
	}

	if u.Scheme == "" {
		u, err = discoverSMTP(u.Host)
		if err != nil {
			s.e.Logger.Printf("Failed to discover SMTP server: %v", err)
			return nil
		}
	}

	switch u.Scheme {
	case "smtps":
		s.smtp.tls = true
	case "smtp+insecure":
		s.smtp.insecure = true
	}

	s.smtp.host = u.Host
	if !strings.ContainsRune(s.smtp.host, ':') {
		if u.Scheme == "smtps" {
			s.smtp.host += ":465"
		} else {
			s.smtp.host += ":587"
		}
	}

	c, err := s.dialSMTP()
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	c.Close()

	s.e.Logger.Printf("Configured upstream SMTP server: %v", u)
	return nil
}

func (s *Server) load() error {
	var plugins []Plugin
	for _, load := range pluginLoaders {
		l, err := load(s)
		if err != nil {
			return fmt.Errorf("failed to load plugins: %v", err)
		}
		for _, p := range l {
			s.e.Logger.Printf("Loaded plugin %q", p.Name())
		}
		plugins = append(plugins, l...)
	}

	renderer := newRenderer(s.e.Logger, s.Options.ThemesPath, s.Options.Theme)
	if err := renderer.Load(plugins); err != nil {
		return fmt.Errorf("failed to load templates: %v", err)
	}

	// Once we've loaded plugins and templates from disk (which can take time),
	// swap them in the Server struct
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Close previous plugins
	for _, p := range s.plugins {
		if err := p.Close(); err != nil {
			s.e.Logger.Printf("Failed to unload plugin %q: %v", p.Name(), err)
		}
	}

	s.plugins = plugins
	s.e.Renderer = renderer

	for _, p := range plugins {
		p.SetRoutes(s.e.Group(""))
	}

	return nil
}

// Reload loads Lua plugins and templates from disk.
func (s *Server) Reload() error {
	s.e.Logger.Printf("Reloading server")
	return s.load()
}

// Logger returns this server's logger.
func (s *Server) Logger() echo.Logger {
	return s.e.Logger
}

// Context is the context used by HTTP handlers.
//
// Use a type assertion to get it from a echo.Context:
//
//	ctx := ectx.(*alps.Context)
type Context struct {
	echo.Context
	Server  *Server
	Session *Session // nil if user isn't logged in
}

var aLongTimeAgo = time.Unix(233431200, 0)

// SetSession sets a cookie for the provided session. Passing a nil session
// unsets the cookie.
func (ctx *Context) SetSession(s *Session) {
	cookie := http.Cookie{
		Name:     cookieName,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   ctx.IsTLS(),
	}
	if s != nil {
		cookie.Value = s.token
	} else {
		cookie.Expires = aLongTimeAgo // unset the cookie
	}
	ctx.SetCookie(&cookie)
}

type loginToken struct {
	Username string
	Password string
}

func (ctx *Context) SetLoginToken(username, password string) {
	cookie := http.Cookie{
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		Name:     loginTokenCookieName,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   ctx.IsTLS(),
		Path:     "/login",
	}
	if username == "" {
		cookie.Expires = aLongTimeAgo // unset the cookie
		ctx.SetCookie(&cookie)
		return
	}

	loginToken := loginToken{username, password}
	payload, err := json.Marshal(loginToken)
	if err != nil {
		panic(err) // Should never happen
	}
	fkey := ctx.Server.Options.LoginKey
	if fkey == nil {
		return
	}

	bytes, err := fernet.EncryptAndSign(payload, fkey)
	if err != nil {
		log.Printf("Warning: login token encryption failed: %v", err)
		return
	}

	cookie.Value = string(bytes)
	ctx.SetCookie(&cookie)
}

func (ctx *Context) GetLoginToken() (string, string) {
	cookie, err := ctx.Cookie(loginTokenCookieName)
	if err != nil || cookie == nil {
		return "", ""
	}

	fkey := ctx.Server.Options.LoginKey
	if fkey == nil {
		return "", ""
	}

	bytes := fernet.VerifyAndDecrypt([]byte(cookie.Value),
		24*time.Hour*30, []*fernet.Key{fkey})
	if bytes == nil {
		return "", ""
	}

	var token loginToken
	err = json.Unmarshal(bytes, &token)
	if err != nil {
		panic(err) // Should never happen
	}

	return token.Username, token.Password
}

func isPublic(path string) bool {
	if strings.HasPrefix(path, "/plugins/") {
		parts := strings.Split(path, "/")
		return len(parts) >= 4 && parts[3] == "assets"
	}
	return path == "/login" || strings.HasPrefix(path, "/themes/")
}

func redirectToLogin(ctx *Context) error {
	path := ctx.Request().URL.Path
	to := "/login"
	// redirect loop with /webmail/login
	if path != "/" && path != "/login" {
		to += "?next=" + url.QueryEscape(ctx.Request().URL.String())
	}
	return ctx.Redirect(http.StatusFound, to)
}

func handleUnauthenticated(next echo.HandlerFunc, ctx *Context) error {
	// Require auth for all requests except /login and assets
	if isPublic(ctx.Request().URL.Path) {
		return next(ctx)
	} else {
		return redirectToLogin(ctx)
	}
}

type Options struct {
	Upstreams  []string
	Theme      string
	ThemesPath string
	Debug      bool
	LoginKey   *fernet.Key
}

// New creates a new server.
func New(e *echo.Echo, options *Options) (*Server, error) {
	s, err := newServer(e, options)
	if err != nil {
		return nil, err
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	e.HTTPErrorHandler = func(err error, ctx echo.Context) {
		code := http.StatusInternalServerError
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
		}

		type ErrorRenderData struct {
			BaseRenderData
			Code   int
			Err    error
			Status string
		}
		rdata := ErrorRenderData{
			BaseRenderData: *NewBaseRenderData(ctx),
			Err:            err,
			Code:           code,
			Status:         http.StatusText(code),
		}

		if err := ctx.Render(code, "error.html", &rdata); err != nil {
			ctx.Logger().Error(fmt.Errorf(
				"Error occured rendering error page: %w. How meta.", err))
		}

		ctx.Logger().Error(err)
	}

	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx echo.Context) error {
			s.mutex.RLock()
			err := next(ectx)
			s.mutex.RUnlock()
			return err
		}
	})

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx echo.Context) error {
			// `style-src 'unsafe-inline'` is required for e-mails with
			// embedded stylesheets
			ectx.Response().Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
			// DNS prefetching has privacy implications
			ectx.Response().Header().Set("X-DNS-Prefetch-Control", "off")
			return next(ectx)
		}
	})

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx echo.Context) error {
			ctx := &Context{Context: ectx, Server: s}
			ctx.Set("context", ctx)

			cookie, err := ctx.Cookie(cookieName)
			if err == http.ErrNoCookie {
				return handleUnauthenticated(next, ctx)
			} else if err != nil {
				return err
			}

			ctx.Session, err = ctx.Server.Sessions.get(cookie.Value)
			if err == ErrSessionExpired {
				ctx.SetSession(nil)
				return handleUnauthenticated(next, ctx)
			} else if err != nil {
				return err
			}
			ctx.Session.ping()

			return next(ctx)
		}
	})

	e.Static("/themes", options.ThemesPath)

	return s, nil
}
