package main

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// indexPlaceholder marks the spot in index.html where the server injects the
// title/description meta tags derived from the loaded config.
const indexPlaceholder = "<!-- statuspage:inject -->"

// maxSummaryNames caps how many host patterns a single group line shows before
// collapsing into "+N more".
const maxSummaryNames = 4

// machineSummary returns one line per configured group with its machine count
// and the distinct host patterns it was expanded from (falling back to machine
// names for top-level machines), mirroring the per-group recap printed by
// --check.
func machineSummary(cfg *Config) []string {
	byGroup := map[string]int{}
	patterns := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, m := range cfg.Machines {
		byGroup[m.Group]++
	}
	for _, g := range cfg.Groups {
		seen[g.Name] = map[string]bool{}
		for _, mc := range g.Machines {
			if !seen[g.Name][mc.Host] {
				seen[g.Name][mc.Host] = true
				patterns[g.Name] = append(patterns[g.Name], mc.Host)
			}
		}
	}
	lines := make([]string, 0, len(cfg.Groups)+1)
	for _, g := range cfg.Groups {
		lines = append(lines, formatGroupSummary(g.Name, byGroup[g.Name], patterns[g.Name]))
	}
	if n := byGroup[""]; n > 0 {
		names := []string{}
		s := map[string]bool{}
		for _, m := range cfg.Machines {
			if m.Group == "" && !s[m.Name] {
				s[m.Name] = true
				names = append(names, m.Name)
			}
		}
		lines = append(lines, formatGroupSummary("(top-level)", n, names))
	}
	return lines
}

func formatGroupSummary(name string, n int, items []string) string {
	line := fmt.Sprintf("%s: %d", name, n)
	if len(items) == 0 {
		return line
	}
	show := items
	extra := ""
	if len(items) > maxSummaryNames {
		show = items[:maxSummaryNames]
		extra = fmt.Sprintf(" +%d more", len(items)-maxSummaryNames)
	}
	return fmt.Sprintf("%s (%s%s)", line, strings.Join(show, ", "), extra)
}

func metaTags(title, description string) string {
	if description == "" {
		description = "status page"
	}
	return fmt.Sprintf(
		"<meta name=\"title\" content=\"%s\" />\n    <meta name=\"description\" content=\"%s\" />",
		html.EscapeString(title), html.EscapeString(description),
	)
}

// indexHandler serves the built SPA, replacing indexPlaceholder in index.html
// with config-derived meta tags. All other paths are delegated to the static
// file server.
func indexHandler(dist, title, description string) http.Handler {
	indexPath := filepath.Join(dist, "index.html")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatalf("reading %s: %v", indexPath, err)
	}
	body := strings.Replace(string(index), indexPlaceholder, metaTags(title, description), 1)
	fs := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			fs.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, body)
	})
}
