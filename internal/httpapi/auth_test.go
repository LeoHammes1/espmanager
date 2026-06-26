package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSessions struct {
	created map[string]time.Time
	deleted map[string]bool
	valid   bool
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{created: map[string]time.Time{}, deleted: map[string]bool{}}
}

func (f *fakeSessions) Create(_ context.Context, id string, expiresAt time.Time) error {
	f.created[id] = expiresAt
	return nil
}

func (f *fakeSessions) Valid(_ context.Context, id string, _ time.Time) (bool, error) {
	return f.valid, nil
}

func (f *fakeSessions) Delete(_ context.Context, id string) error {
	f.deleted[id] = true
	return nil
}

func testGuard(t *testing.T, sessions SessionStore, password string) *authGuard {
	t.Helper()
	tmpl, err := ParseTemplates()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return &authGuard{
		sessions: sessions,
		password: password,
		tmpl:     tmpl,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestMiddlewareRedirectsWithoutSession(t *testing.T) {
	g := testGuard(t, newFakeSessions(), "secret")
	called := false
	h := g.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if called {
		t.Fatal("protected handler ran without a session")
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("want redirect to /login, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestMiddlewareAllowsValidSession(t *testing.T) {
	s := newFakeSessions()
	s.valid = true
	g := testGuard(t, s, "secret")
	called := false
	h := g.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("protected handler did not run for a valid session")
	}
}

func TestMiddlewareBlocksWhenNoPasswordConfigured(t *testing.T) {
	s := newFakeSessions()
	s.valid = true
	g := testGuard(t, s, "") // setup required
	called := false
	h := g.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("server must not be reachable with no admin password set")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want redirect, got %d", rec.Code)
	}
}

func TestLoginSucceedsAndSetsCookie(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(t, s, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	g.loginSubmit(rec, req)

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("want redirect to /, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if len(s.created) != 1 {
		t.Fatalf("want 1 session created, got %d", len(s.created))
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" && c.HttpOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("want an HttpOnly session cookie")
	}
}

func TestLoginCookieSecureFollowsConfig(t *testing.T) {
	g := testGuard(t, newFakeSessions(), "secret")
	g.secureCookies = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	g.loginSubmit(rec, req)

	var c *http.Cookie
	for _, cc := range rec.Result().Cookies() {
		if cc.Name == sessionCookie {
			c = cc
		}
	}
	if c == nil || !c.Secure {
		t.Fatal("want a Secure session cookie when secureCookies is enabled")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(t, s, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=nope"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	g.loginSubmit(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if len(s.created) != 0 {
		t.Fatal("no session should be created on a failed login")
	}
}

func TestLoginPageShowsSetupRequired(t *testing.T) {
	g := testGuard(t, newFakeSessions(), "")

	rec := httptest.NewRecorder()
	g.loginPage(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if !strings.Contains(rec.Body.String(), "ESPM_ADMIN_PASSWORD") {
		t.Fatal("setup-required login page should mention ESPM_ADMIN_PASSWORD")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(t, s, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	g.logout(rec, req)

	if !s.deleted["abc"] {
		t.Fatal("logout should delete the session")
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("want redirect to /login, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
