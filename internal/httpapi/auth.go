package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/LeoHammes1/espmanager/internal/httpx"
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
	user          string
	password      string
	secureCookies bool
	log           *slog.Logger
}

type sessionState struct {
	Authenticated bool   `json:"authenticated"`
	SetupRequired bool   `json:"setupRequired"`
	User          string `json:"user"`
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

func (a *authGuard) state(r *http.Request) sessionState {
	authed := a.authenticated(r)
	user := ""
	if authed {
		user = a.user
	}
	return sessionState{Authenticated: authed, SetupRequired: a.password == "", User: user}
}

// requireAPI guards JSON endpoints: an unauthenticated request gets a 401 JSON
// body (never a redirect), so the SPA's fetch layer can treat it as "session
// expired" and route to the login screen.
func (a *authGuard) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		apiErr(w, http.StatusUnauthorized, "unauthorized", "Sign in to continue.")
	})
}

func (a *authGuard) getSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, a.state(r))
}

func (a *authGuard) postSession(w http.ResponseWriter, r *http.Request) {
	if a.password == "" {
		apiErr(w, http.StatusConflict, "setup_required", "Set ESPM_ADMIN_PASSWORD and restart the server.")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid_request", "Malformed request.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.password)) != 1 {
		apiErr(w, http.StatusUnauthorized, "invalid_credentials", "Wrong password.")
		return
	}

	sid, err := id.New(24)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := a.sessions.Create(r.Context(), sid, expires); err != nil {
		a.log.Error("create session failed", "err", err)
		apiErr(w, http.StatusInternalServerError, "internal", "Something went wrong.")
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
	httpx.WriteJSON(w, http.StatusOK, sessionState{Authenticated: true, SetupRequired: false, User: a.user})
}

func (a *authGuard) deleteSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := a.sessions.Delete(r.Context(), c.Value); err != nil {
			a.log.Error("delete session failed", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
