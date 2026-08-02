package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func printConfigSummary(path string, cfg *Config) {
	log.Printf("config %s ok: %d machines", path, len(cfg.Machines))
	for _, line := range machineSummary(cfg) {
		log.Printf("  %s", line)
	}
}

func buildFrontend() error {
	cmd := "npm"
	if _, err := exec.LookPath("npm"); err != nil {
		cmd = "bun"
	}
	c := exec.Command(cmd, "run", "build")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	log.Printf("running %s run build", cmd)
	return c.Run()
}

func main() {
	_ = godotenv.Load()

	dev := pflag.Bool("dev", false, "run `npm run build` before serving dist/")
	devServer := pflag.Bool("dev-server", false, "frontend served by the vite dev server, do not serve dist/")
	mock := pflag.Bool("mock", false, "serve generated mock data instead of probing real machines")
	check := pflag.Bool("check", false, "load and expand the config, print a machine summary, then exit")
	addr := pflag.String("addr", envOr("ADDR", ":5000"), "listen address")
	configPath := pflag.StringP("config", "c", envOr("CONFIG", "config.local.yaml"), "yaml config file")
	dbPath := pflag.String("db", envOr("DB", "data.volume/history.db"), "sqlite history database file")
	pflag.Parse()

	if *check {
		cfg, err := LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("config check failed: %v", err)
		}
		printConfigSummary(*configPath, cfg)
		return
	}

	if *dev {
		if err := buildFrontend(); err != nil {
			log.Fatalf("build failed: %v", err)
		}
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	log.Printf("loaded %d machines from %s", len(cfg.Machines), *configPath)
	title := cfg.Title
	desc := strings.Join(machineSummary(cfg), ", ")

	mux := http.NewServeMux()

	if *mock {
		log.Printf("mock mode: serving generated data (no real probes)")
		mm := NewMockMonitor()
		mux.HandleFunc("GET /api/status", mm.statusHandler)
		mux.HandleFunc("GET /api/history/{machine}", mm.historyHandler)
		mux.HandleFunc("GET /api/metrics/aggregate", mm.metricsAggregateHandler)
		mux.HandleFunc("GET /api/metrics/{machine}", mm.metricsHandler)
		mux.HandleFunc("POST /api/refresh/{machine}", mm.refreshHandler)
		if !*devServer {
			mux.Handle("GET /", indexHandler("dist", title, desc))
		}
		log.Printf("listening on %s (mock)", *addr)
		if err := http.ListenAndServe(*addr, mux); err != nil {
			log.Fatal(err)
		}
		return
	}

	if dir := filepath.Dir(*dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("creating db dir: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	history, err := NewHistory(*dbPath)
	if err != nil {
		log.Fatalf("opening history db: %v", err)
	}
	defer history.Close()
	go history.pruneLoop(ctx)

	mon := NewMonitor(cfg, history)
	mon.Start(ctx)

	mux.HandleFunc("GET /api/status", mon.statusHandler)
	mux.HandleFunc("GET /api/history/{machine}", mon.historyHandler)
	mux.HandleFunc("GET /api/metrics/aggregate", mon.metricsAggregateHandler)
	mux.HandleFunc("GET /api/metrics/{machine}", mon.metricsHandler)
	mux.HandleFunc("POST /api/refresh/{machine}", mon.refreshHandler)
	if !*devServer {
		mux.Handle("GET /", indexHandler("dist", title, desc))
	}

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
