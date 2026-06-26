package httpapi

import (
	"context"
	"encoding/json"
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
func (f *fakeSessions) Valid(_ context.Context, _ string, _ time.Time) (bool, error) {
	return f.valid, nil
}
func (f *fakeSessions) Delete(_ context.Context, id string) error {
	f.deleted[id] = true
	return nil
}

func testGuard(sessions SessionStore, password string) *authGuard {
	return &authGuard{
		sessions: sessions,
		user:     "admin",
		password: password,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func decode[T any](t *testing.T, body io.Reader) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestRequireAPIRejectsWithoutSession(t *testing.T) {
	g := testGuard(newFakeSessions(), "secret")
	called := false
	h := g.requireAPI(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/devices", nil))

	if called {
		t.Fatal("guarded handler ran without a session")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if got := decode[map[string]string](t, rec.Body)["error"]; got != "unauthorized" {
		t.Fatalf("want error=unauthorized, got %q", got)
	}
}

func TestRequireAPIAllowsValidSession(t *testing.T) {
	s := newFakeSessions()
	s.valid = true
	g := testGuard(s, "secret")
	called := false
	h := g.requireAPI(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !called {
		t.Fatal("guarded handler did not run for a valid session")
	}
}

func TestGetSessionReportsState(t *testing.T) {
	g := testGuard(newFakeSessions(), "secret")
	rec := httptest.NewRecorder()
	g.getSession(rec, httptest.NewRequest(http.MethodGet, "/api/session", nil))

	st := decode[sessionState](t, rec.Body)
	if st.Authenticated || st.SetupRequired {
		t.Fatalf("want unauthenticated, not setup-required, got %+v", st)
	}
}

func TestGetSessionSetupRequired(t *testing.T) {
	g := testGuard(newFakeSessions(), "")
	rec := httptest.NewRecorder()
	g.getSession(rec, httptest.NewRequest(http.MethodGet, "/api/session", nil))

	if !decode[sessionState](t, rec.Body).SetupRequired {
		t.Fatal("want setupRequired=true when no admin password is set")
	}
}

func TestPostSessionSucceeds(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(s, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"password":"secret"}`))
	g.postSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if len(s.created) != 1 {
		t.Fatalf("want 1 session created, got %d", len(s.created))
	}
	var hasCookie bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" && c.HttpOnly {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Fatal("want an HttpOnly session cookie")
	}
}

func TestPostSessionRejectsWrongPassword(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(s, "secret")
	rec := httptest.NewRecorder()
	g.postSession(rec, httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"password":"nope"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if len(s.created) != 0 {
		t.Fatal("no session should be created on a failed login")
	}
}

func TestPostSessionSetupRequired(t *testing.T) {
	g := testGuard(newFakeSessions(), "")
	rec := httptest.NewRecorder()
	g.postSession(rec, httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"password":"x"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409 setup_required, got %d", rec.Code)
	}
}

func TestDeleteSessionClears(t *testing.T) {
	s := newFakeSessions()
	g := testGuard(s, "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	g.deleteSession(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if !s.deleted["abc"] {
		t.Fatal("logout should delete the session")
	}
}
