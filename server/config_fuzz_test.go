package main

import (
	"reflect"
	"strings"
	"testing"
)

func FuzzExpandHostPattern(f *testing.F) {
	seeds := []string{
		"server{1..3}.example.org",
		"server{1,2,3}.example.org",
		"server-{a3,a4}-{1..2}.example.org",
		"host{01..03}",
		"router.example.net",
		"host-{5..2}.example.org",
		"host-{1,2.example.org",
		"{1..10}",
		"{2..5,8..10}.example.org",
		"",
		"host-{}",
		"{1..}",
		"{..5}",
		"x}{y}",
		"{a,b}{c,d}",
		"{0..10001}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		hosts, err := expandHostPattern(pattern)
		if err != nil {
			return
		}
		if !strings.Contains(pattern, "{") {
			if !reflect.DeepEqual(hosts, []string{pattern}) {
				t.Fatalf("pattern without braces %q expanded to %v", pattern, hosts)
			}
			return
		}
		if len(hosts) == 0 {
			t.Fatalf("pattern %q produced no hosts", pattern)
		}
		for _, h := range hosts {
			if strings.Contains(h, "{") {
				t.Fatalf("expanded host %q still contains '{' (pattern %q)", h, pattern)
			}
		}
	})
}
