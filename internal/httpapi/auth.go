package httpapi

import (
	"context"
	"crypto/subtle"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeoHammes1/espmanager/internal/id"
)

const (
	sessionCookie = "espm_session"
	sessionTTL    = 7 * 24 * time.Hour
)

type SessionStore interface {
	Create(ctx context.Context, id string, expiresAt time.Time) error
	Valid(ctx context.Context, id string, now time.Time) (bool, error)
	Delete(ctx context.Context, id string) error
}

type authGuard struct {
	sessions      SessionStore
	password      string
	secureCookies bool
	tmpl          *template.Template
	log           *slog.Logger
}

type loginView struct {
	SetupRequired bool
	Error         string
}

// middleware guards the in-shell routes. With no admin password configured the
// server is unprotected by design today; instead of silently letting everyone
// through (the old BasicAuth behaviour), it forces the login screen, which
// explains the setup-required state.
func (a *authGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

func (a *authGuard) authenticated(r *http.Request) bool {
	if a.password == "" {
		return false
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	ok, err := a.sessions.Valid(r.Context(), c.Value, time.Now())
	return err == nil && ok
}

func (a *authGuard) loginPage(w http.ResponseWriter, r *http.Request) {
	if a.authenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.renderLogin(w, http.StatusOK, loginView{SetupRequired: a.password == ""})
}

func (a *authGuard) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if a.password == "" {
		a.renderLogin(w, http.StatusOK, loginView{SetupRequired: true})
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte(a.password)) != 1 {
		a.renderLogin(w, http.StatusUnauthorized, loginView{Error: "Wrong password. Try again."})
		return
	}

	sid, err := id.New(24)
	if err != nil {
		a.renderLogin(w, http.StatusInternalServerError, loginView{Error: "Something went wrong. Try again."})
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := a.sessions.Create(r.Context(), sid, expires); err != nil {
		a.log.Error("create session failed", "err", err)
		a.renderLogin(w, http.StatusInternalServerError, loginView{Error: "Something went wrong. Try again."})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secureCookies,
		Expires:  expires,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *authGuard) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := a.sessions.Delete(r.Context(), c.Value); err != nil {
			a.log.Error("delete session failed", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *authGuard) renderLogin(w http.ResponseWriter, status int, v loginView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.tmpl.ExecuteTemplate(w, "login", v); err != nil {
		a.log.Error("render login failed", "err", err)
	}
}
