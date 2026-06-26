package httpapi

import (
	"net/http"
	"strings"

	"github.com/LeoHammes1/espmanager/internal/firmware"
)

// agentFirmware serves the embedded base firmware (manifest.json + the per-chip
// binaries) for the browser onboarding wizard to flash. It is public: the agent
// firmware carries no secrets (WiFi/host/credential are provisioned over serial),
// and esp-web-tools/esptool-js fetch it without credentials.
func agentFirmware() (http.Handler, error) {
	sub, err := firmware.Agent()
	if err != nil {
		return nil, err
	}
	fs := http.FileServer(http.FS(sub))
	return http.StripPrefix("/firmware/agent/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})), nil
}
