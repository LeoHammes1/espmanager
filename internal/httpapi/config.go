package httpapi

import (
	"net/http"

	"github.com/LeoHammes1/espmanager/internal/httpx"
)

// apiConfig exposes the device-reachable manager address so the onboarding
// wizard provisions the authoritative endpoint instead of guessing from the
// browser origin.
func apiConfig(p ProvisionInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}
