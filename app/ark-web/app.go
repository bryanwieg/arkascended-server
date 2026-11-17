package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
	"pkg/ark/sdkbackend"

	"ark-web/internal/server"
)

type WebApp struct {
	srv *server.Server
}

func NewWebApp(deps *server.Deps) *WebApp {
	return &WebApp{srv: server.New(deps)}
}

func (w *WebApp) Run(addr string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("ARK web server starting")

	var runtimeLabel string
	if _, ok := host.HasDockerService(); ok {
		runtimeLabel = "docker"
	} else if socketURI, rootfulDetected := host.HasPodmanService(); rootfulDetected {
		return fmt.Errorf("rootful podman socket detected; only rootless podman is supported")
	} else if socketURI != "" {
		if err := os.Setenv("DOCKER_HOST", socketURI); err != nil {
			return fmt.Errorf("failed to set DOCKER_HOST: %v", err)
		}
		runtimeLabel = "podman"
		logger.Info("using podman socket", "uri", socketURI)
	} else {
		return fmt.Errorf("no container runtime socket found (expected /var/run/docker.sock or podman socket)")
	}
	logger.Info("container runtime resolved", "runtime", runtimeLabel)

	sdkEng, sdkErr := sdkbackend.NewSDKEngine(logger)
	if sdkErr != nil {
		return fmt.Errorf("SDK engine init: %w", sdkErr)
	}
	sdkComp, sdkErr := sdkbackend.NewSDKCompose(logger)
	if sdkErr != nil {
		return fmt.Errorf("SDK compose init: %w", sdkErr)
	}

	if err := sdkEng.ValidateDaemonVersion(ctx, runtimeLabel); err != nil {
		return err
	}

	hc := health.New(sdkComp, logger)
	lc := lifecycle.New(sdkComp, hc, logger)

	deps := &server.Deps{
		Engine:         sdkEng,
		ComposeService: sdkComp,
		HealthChecker:  hc,
		Lifecycle:      lc,
		BackupService:  nil,
		UpdateService:  nil,
		Logger:         logger,
	}

	w = NewWebApp(deps)
	return w.srv.ListenAndServe(addr)
}

func Main() {
	addr := os.Getenv("ARK_WEB_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	app := &WebApp{}
	if err := app.Run(addr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
