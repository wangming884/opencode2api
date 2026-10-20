// Command opencode2api starts the API gateway and its management interface.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminui "opencode2api/internal/admin"
	"opencode2api/internal/buildinfo"
	"opencode2api/internal/config"
	"opencode2api/internal/gateway"
	"opencode2api/internal/telemetry"
)

// version remains the linker injection point used by release builds.
var version = "dev"

// main loads the configuration, starts the gateway runtime and the API and
// WebUI servers, and shuts them down on SIGINT or SIGTERM.
func main() {
	buildinfo.Version = version
	configPath := flag.String("config", "config.json", "path to config.json")
	listen := flag.String("listen", "", "override the configured API listen address")
	webListen := flag.String("web-listen", "", "override the configured WebUI listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *webListen != "" {
		cfg.WebUI.Listen = *webListen
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	level := new(slog.LevelVar)
	telemetry.SetLogLevel(level, cfg.Logging.Level)
	hub := telemetry.LogHub(cfg.Logging.RingSize)
	redactor := config.SecretRedactor()
	redactor.Replace(cfg)
	logger := telemetry.StructuredLogger(level, hub, redactor)
	monitor := telemetry.Monitor()
	manager, err := gateway.RuntimeManager(ctx, *configPath, cfg, logger, monitor, hub, redactor, level)
	if err != nil {
		logger.Error("failed to initialize runtime", "component", "runtime", "event", "runtime_initialization_failed", "error", err)
		os.Exit(1)
	}
	defer manager.Shutdown()

	var webServer *http.Server
	apiHandler := manager.Handler()
	if cfg.WebUI.Enabled {
		admin := adminui.New(manager, monitor, hub, logger)
		if sameListenPort(cfg.Listen, cfg.WebUI.Listen) {
			// A single public port is enough for both surfaces: keep the API
			// routes exact and send every other request to the WebUI. This
			// also avoids trying to bind the same address twice.
			apiHandler = combinedHandler(apiHandler, admin.Handler())
		} else {
			webServer = &http.Server{
				Addr: cfg.WebUI.Listen, Handler: admin.Handler(), ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second,
			}
		}
	}
	apiServer := &http.Server{
		Addr: cfg.Listen, Handler: apiHandler, ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second,
	}
	servers := []*http.Server{apiServer}
	go serveHTTP(cancel, logger, apiServer, "api")

	if webServer != nil {
		servers = append(servers, webServer)
		go serveHTTP(cancel, logger, webServer, "webui")
	}

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "component", "server", "event", "shutdown_failed", "address", server.Addr, "error", err)
		}
	}
}

// sameListenPort reports whether two listener addresses use the same TCP port.
// Exact matches are accepted even when the address is not in host:port form.
func sameListenPort(first, second string) bool {
	if first == second {
		return true
	}
	_, firstPort, firstErr := net.SplitHostPort(first)
	_, secondPort, secondErr := net.SplitHostPort(second)
	return firstErr == nil && secondErr == nil && firstPort == secondPort
}

// combinedHandler exposes the gateway API and the management WebUI on one
// listener. The gateway owns /v1, /metrics and /healthz, so all other paths
// fall through to the admin handler and its embedded SPA.
func combinedHandler(api, webui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1", api)
	mux.Handle("/v1/", api)
	mux.Handle("/metrics", api)
	mux.Handle("/healthz", api)
	mux.Handle("/", webui)
	return mux
}

// serveHTTP serves server until it is shut down. An unexpected stop is logged
// and cancels the root context, which begins the shutdown of every server.
func serveHTTP(cancel context.CancelFunc, logger *slog.Logger, server *http.Server, component string) {
	logger.Info("server listening", "component", component, "event", "server_started", "address", server.Addr, "version", buildinfo.Version)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped unexpectedly", "component", component, "event", "server_failed", "address", server.Addr, "error", err)
		cancel()
	}
}
