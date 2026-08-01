package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpandHostPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
		err     string
	}{
		{
			pattern: "server{1..3}.example.org",
			want:    []string{"server1.example.org", "server2.example.org", "server3.example.org"},
		},
		{
			pattern: "server{1,2,3}.example.org",
			want:    []string{"server1.example.org", "server2.example.org", "server3.example.org"},
		},
		{
			pattern: "server{2..5,8..10}.example.org",
			want: []string{
				"server2.example.org", "server3.example.org", "server4.example.org",
				"server5.example.org", "server8.example.org", "server9.example.org", "server10.example.org",
			},
		},
		{
			pattern: "server-{a3,a4}-{1..2}.example.org",
			want: []string{
				"server-a3-1.example.org",
				"server-a3-2.example.org",
				"server-a4-1.example.org",
				"server-a4-2.example.org",
			},
		},
		{
			pattern: "node{01..03}",
			want:    []string{"node01", "node02", "node03"},
		},
		{
			pattern: "router.example.net",
			want:    []string{"router.example.net"},
		},
		{
			pattern: "host-{1..2}-{a,b}",
			want:    []string{"host-1-a", "host-1-b", "host-2-a", "host-2-b"},
		},
		{
			pattern: "{1..10..2}.example.org",
			err:     "endpoints must be numbers",
		},
		{
			pattern: "host-{5..2}.example.org",
			err:     "5 > 2",
		},
		{
			pattern: "host-{x..y}.example.org",
			err:     "endpoints must be numbers",
		},
		{
			pattern: "host-{1,2.example.org",
			err:     "unmatched '{'",
		},
		{
			pattern: "host-{1..2-{3..4}}.example.org",
			err:     "nested",
		},
		{
			pattern: "{{",
			err:     "unmatched '{'",
		},
		{
			pattern: "{{1,2}}",
			err:     "nested",
		},
		{
			pattern: "{a}{{b}}",
			err:     "nested",
		},
		{
			pattern: "{1..5..10}",
			err:     "endpoints must be numbers",
		},
		{
			pattern: "{1...5}",
			err:     "endpoints must be numbers",
		},
		{
			pattern: "{1..2..3}",
			err:     "endpoints must be numbers",
		},
		{
			pattern: "{1..2.5}",
			err:     "endpoints must be numbers",
		},
	}

	for _, tt := range tests {
		got, err := expandHostPattern(tt.pattern)
		if tt.err != "" {
			if err == nil {
				t.Errorf("%q: expected error containing %q, got nil", tt.pattern, tt.err)
			} else if !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%q: expected error containing %q, got %q", tt.pattern, tt.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.pattern, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q:\n got %v\nwant %v", tt.pattern, got, tt.want)
		}
	}
}

func TestStatusPayloadExpandsGlobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := `
groups:
  - name: Aula 3
    machines:
      - host: a3-dott{1..3}.cs.dm.unipi.it
machines:
  - host: router.example.net
`
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	conf, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	mon := NewMonitor(conf, nil)
	p := mon.StatusPayload()

	if len(p.Groups) != 1 {
		t.Fatalf("want 1 group, got %d", len(p.Groups))
	}
	g := p.Groups[0]
	want := []string{
		"a3-dott1.cs.dm.unipi.it",
		"a3-dott2.cs.dm.unipi.it",
		"a3-dott3.cs.dm.unipi.it",
	}
	if len(g.Machines) != len(want) {
		t.Fatalf("group machines: want %d, got %d", len(want), len(g.Machines))
	}
	for i, m := range g.Machines {
		if m.Host != want[i] {
			t.Errorf("machine %d: host %q, want %q", i, m.Host, want[i])
		}
	}
	if len(p.Machines) != 1 {
		t.Fatalf("top-level machines: want 1, got %d", len(p.Machines))
	}
	if p.Machines[0].Host != "router.example.net" {
		t.Errorf("top-level host: %q", p.Machines[0].Host)
	}
}
