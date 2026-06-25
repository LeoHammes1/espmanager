package main

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/LeoHammes1/espmanager/internal/artifact"
	"github.com/LeoHammes1/espmanager/internal/config"
	"github.com/LeoHammes1/espmanager/internal/device"
	"github.com/LeoHammes1/espmanager/internal/driver"
	"github.com/LeoHammes1/espmanager/internal/httpapi"
	"github.com/LeoHammes1/espmanager/internal/mqttbroker"
	"github.com/LeoHammes1/espmanager/internal/queue"
	"github.com/LeoHammes1/espmanager/internal/signclient"
	sqlitestore "github.com/LeoHammes1/espmanager/internal/store/sqlite"
	"github.com/LeoHammes1/espmanager/internal/web"
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
	jobs := queue.New(db, "builds")

	if cfg.AdminPassword == "" {
		log.Warn("management UI is unauthenticated; set ESPM_ADMIN_PASSWORD to require login")
	}

	broker, err := mqttbroker.New(cfg.MQTTAddr, deviceSvc)
	if err != nil {
		return err
	}
	if err := broker.Start(); err != nil {
		return err
	}
	defer broker.Close()
	log.Info("mqtt broker listening", "addr", cfg.MQTTAddr)

	tmpl, err := template.ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return err
	}

	router, err := httpapi.NewRouter(httpapi.Options{
		Devices:       deviceSvc,
		Drivers:       driverSvc,
		Artifacts:     artifactSvc,
		Hub:           hub,
		Templates:     tmpl,
		Queue:         jobs,
		Webhook:       webhook.NewHandler(driverSvc, jobs, log),
		WorkerToken:   cfg.WorkerToken,
		AdminUser:     cfg.AdminUser,
		AdminPassword: cfg.AdminPassword,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
