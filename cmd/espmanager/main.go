package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/config"
	"github.com/LeoHammes1/espmanager/internal/deploy"
	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/enroll"
	"github.com/LeoHammes1/espmanager/internal/httpapi"
	"github.com/LeoHammes1/espmanager/internal/mqttbroker"
	"github.com/LeoHammes1/espmanager/internal/queue"
	"github.com/LeoHammes1/espmanager/internal/signclient"
	sqlitestore "github.com/LeoHammes1/espmanager/internal/store/sqlite"
	"github.com/LeoHammes1/espmanager/internal/topics"
	"github.com/LeoHammes1/espmanager/internal/webhook"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return err
	}

	db, err := sqlitestore.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	hub := httpapi.NewSSEHub()
	driverSvc := driver.NewService(sqlitestore.NewDriverRepository(db))
	deviceSvc := device.NewService(sqlitestore.NewDeviceRepository(db), driverSvc, hub, log)
	signer := signclient.New(cfg.SignerURL, cfg.SignerToken, nil)
	artifactSvc := artifact.NewService(sqlitestore.NewArtifactRepository(db), signer, driverSvc, cfg.ArtifactsDir)
	enrollSvc := enroll.NewService(sqlitestore.NewEnrollRepository(db), cfg.ClaimTTL)
	jobs := queue.New(db, "builds", cfg.BuildTimeout)

	if cfg.AdminPassword == "" {
		log.Warn("management UI is unauthenticated; set ESPM_ADMIN_PASSWORD to require login")
	}
	if err := validatePublicURL(cfg.PublicURL); err != nil {
		return err
	}
	if cfg.PublicURL == "" {
		log.Warn("OTA rollouts disabled; set ESPM_PUBLIC_URL to the device-reachable base URL")
	}
	if err := validateBrowserURL(cfg.BrowserURL); err != nil {
		return err
	}
	if !strings.HasPrefix(cfg.BrowserURL, "https://") {
		log.Warn("session cookies will not be marked Secure; set ESPM_BROWSER_URL to the https browser origin (terminated by Caddy)")
	}

	if err := deviceSvc.ClearPresence(context.Background()); err != nil {
		return err
	}

	broker, err := mqttbroker.New(cfg.MQTTAddr, deviceSvc, enrollSvc)
	if err != nil {
		return err
	}
	if err := broker.Start(); err != nil {
		return err
	}
	defer broker.Close()
	log.Info("mqtt broker listening", "addr", cfg.MQTTAddr)

	deploySvc := deploy.NewService(sqlitestore.NewDeployRepository(db), deviceSvc, artifactSvc, broker, hub, cfg.PublicURL,
		deploy.Options{
			CanaryPercent:    cfg.CanaryPercent,
			FailureThreshold: cfg.FailureThreshold,
			TargetTimeout:    cfg.TargetTimeout,
		}, log)

	if err := subscribeTelemetry(broker, deviceSvc, deploySvc); err != nil {
		return err
	}

	router, err := httpapi.NewRouter(httpapi.Options{
		Devices:          deviceSvc,
		Drivers:          driverSvc,
		Artifacts:        artifactSvc,
		Deployer:         deploySvc,
		Deploys:          deploySvc,
		Enroller:         enrollSvc,
		Bus:              broker,
		Hub:              hub,
		Queue:            jobs,
		Webhook:          webhook.NewHandler(driverSvc, jobs, log),
		Sessions:         sqlitestore.NewSessionRepository(db),
		Log:              log,
		WorkerToken:      cfg.WorkerToken,
		AdminUser:        cfg.AdminUser,
		AdminPassword:    cfg.AdminPassword,
		SecureCookies:    strings.HasPrefix(cfg.BrowserURL, "https://"),
		FailureThreshold: cfg.FailureThreshold,
		PublicURL:        cfg.PublicURL,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(cfg.ReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deploySvc.Reconcile(ctx)
			}
		}
	}()

	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func subscribeTelemetry(broker *mqttbroker.Broker, devices *device.Service, deploys *deploy.Service) error {
	if err := broker.Subscribe(topics.StateFilter(), func(topic string, payload []byte) {
		id, ok := topics.DeviceFromTopic(topic)
		if !ok {
			return
		}
		devices.Heartbeat(id, parseVersion(payload))
	}); err != nil {
		return err
	}

	return broker.Subscribe(topics.OTAStatusFilter(), func(topic string, payload []byte) {
		id, ok := topics.DeviceFromTopic(topic)
		if !ok {
			return
		}
		deploys.OnStatus(context.Background(), id, payload)
	})
}

func validatePublicURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("invalid ESPM_PUBLIC_URL %q: must be an absolute http:// URL — devices fetch OTA over plain HTTP and cannot do TLS; put the browser's https origin on ESPM_BROWSER_URL", raw)
	}
	return nil
}

func validateBrowserURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("invalid ESPM_BROWSER_URL %q: must be an absolute https:// URL (the browser-facing origin terminated by Caddy)", raw)
	}
	return nil
}

func parseVersion(payload []byte) string {
	var msg struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(payload, &msg)
	return msg.Version
}
