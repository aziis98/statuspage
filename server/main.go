package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

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

func spaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		rel := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if rel != "." && rel != "" {
			p := filepath.Join("dist", rel)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, p)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join("dist", "index.html"))
	}
}

func main() {
	dev := pflag.Bool("dev", false, "run `npm run build` before serving dist/")
	devServer := pflag.Bool("dev-server", false, "frontend served by the vite dev server, do not serve dist/")
	addr := pflag.String("addr", ":5000", "listen address")
	configPath := pflag.StringP("config", "c", "config.local.yaml", "yaml config file")
	dbPath := pflag.String("db", "data.volume/history.db", "sqlite history database file")
	pflag.Parse()

	if dir := filepath.Dir(*dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("creating db dir: %v", err)
		}
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", mon.statusHandler)
	mux.HandleFunc("GET /api/history/{machine}", mon.historyHandler)
	mux.HandleFunc("GET /api/metrics/aggregate", mon.metricsAggregateHandler)
	mux.HandleFunc("GET /api/metrics/{machine}", mon.metricsHandler)
	mux.HandleFunc("POST /api/refresh/{machine}", mon.refreshHandler)
	if !*devServer {
		mux.HandleFunc("GET /", spaHandler())
	}

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
