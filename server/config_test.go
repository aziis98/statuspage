package main

import (
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
