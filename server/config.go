package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

type PingConfig struct {
	Interval Duration `yaml:"interval"`
	Timeout  Duration `yaml:"timeout"`
	TCPPort  int      `yaml:"tcp_port"`
}

type SSHConfig struct {
	Interval Duration `yaml:"interval"`
	User     string   `yaml:"user"`
	Port     int      `yaml:"port"`
	Key      string   `yaml:"key"`
	Script   string   `yaml:"script"`
}

type MachineConfig struct {
	Name string      `yaml:"name"`
	Host string      `yaml:"host"`
	Ping *PingConfig `yaml:"ping"`
	SSH  *SSHConfig  `yaml:"ssh"`
}

type GroupConfig struct {
	Name     string          `yaml:"name"`
	Machines []MachineConfig `yaml:"machines"`
}

type MetricRange struct {
	Min *float64 `yaml:"min"`
	Max *float64 `yaml:"max"`
}

type RawConfig struct {
	Title               string                 `yaml:"title"`
	Interactive         bool                   `yaml:"interactive"`
	SharedMetricWindow  bool                   `yaml:"shared_metric_window"`
	Ping                *PingConfig            `yaml:"ping"`
	SSH                 *SSHConfig             `yaml:"ssh"`
	Metrics             map[string]MetricRange `yaml:"metrics"`
	Groups              []GroupConfig          `yaml:"groups"`
	Machines            []MachineConfig        `yaml:"machines"`
}

type Machine struct {
	ID    string
	Name  string
	Host  string
	Group string
	Ping  PingConfig
	SSH   SSHConfig
}

type Config struct {
	Title              string
	Interactive        bool
	SharedMetricWindow bool
	Ping               PingConfig
	SSH                SSHConfig
	Metrics            map[string]MetricRange
	Groups             []GroupConfig
	Machines           []Machine
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw RawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	cfg := &Config{
		Title:              raw.Title,
		Interactive:        raw.Interactive,
		SharedMetricWindow: raw.SharedMetricWindow,
		Ping: PingConfig{
			Interval: Duration{time.Second * 10},
			Timeout:  Duration{time.Second * 2},
			TCPPort:  22,
		},
		SSH: SSHConfig{
			Interval: Duration{time.Minute * 10},
			User:     "root",
			Port:     22,
		},
	}
	if raw.Ping != nil {
		if raw.Ping.Interval.Duration > 0 {
			cfg.Ping.Interval = raw.Ping.Interval
		}
		if raw.Ping.Timeout.Duration > 0 {
			cfg.Ping.Timeout = raw.Ping.Timeout
		}
		if raw.Ping.TCPPort != 0 {
			cfg.Ping.TCPPort = raw.Ping.TCPPort
		}
	}
	if raw.SSH != nil {
		if raw.SSH.Interval.Duration > 0 {
			cfg.SSH.Interval = raw.SSH.Interval
		}
		if raw.SSH.User != "" {
			cfg.SSH.User = raw.SSH.User
		}
		if raw.SSH.Port != 0 {
			cfg.SSH.Port = raw.SSH.Port
		}
		cfg.SSH.Key = raw.SSH.Key
		cfg.SSH.Script = raw.SSH.Script
	}
	if cfg.Title == "" {
		cfg.Title = "Status Page"
	}

	cfg.Groups = raw.Groups
	cfg.Metrics = raw.Metrics
	for _, g := range raw.Groups {
		for _, mc := range g.Machines {
			ms, err := expandMachine(mc, g.Name, cfg)
			if err != nil {
				return nil, err
			}
			cfg.Machines = append(cfg.Machines, ms...)
		}
	}
	for _, mc := range raw.Machines {
		ms, err := expandMachine(mc, "", cfg)
		if err != nil {
			return nil, err
		}
		cfg.Machines = append(cfg.Machines, ms...)
	}
	return cfg, nil
}

// expandMachine expands a MachineConfig's host pattern into one or more
// concrete Machine entries. A host may contain brace groups that are expanded
// cartesian-style, e.g. "server-{a3,a4}-{1..10}.example.org".
func expandMachine(mc MachineConfig, group string, cfg *Config) ([]Machine, error) {
	hosts, err := expandHostPattern(mc.Host)
	if err != nil {
		return nil, err
	}
	machines := make([]Machine, 0, len(hosts))
	for _, host := range hosts {
		m := resolveMachine(mc, group, cfg)
		m.Host = host
		m.ID = host
		if mc.Name == "" {
			m.Name = host
		}
		machines = append(machines, m)
	}
	return machines, nil
}

// maximum number of machines a single host pattern may expand to
const maxHostExpansion = 10000

// expandHostPattern expands brace groups in a host pattern. Supported syntax:
//
//	{1,2,3}          enumeration
//	{2..5,8..10}     inclusive numeric ranges
//	{a3,a4}          labels (any non-range item is taken literally)
//
// Multiple groups expand in cartesian fashion:
//
//	server-{a3,a4}-{1..2}.example.org
//	-> server-a3-1.example.org, server-a3-2.example.org, server-a4-1.example.org, ...
//
// A pattern without braces is returned unchanged.
func expandHostPattern(pattern string) ([]string, error) {
	type segment struct {
		prefix string
		values []string
	}
	var segs []segment
	rest := pattern
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			if rest != "" {
				segs = append(segs, segment{prefix: rest})
			}
			break
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			return nil, fmt.Errorf("host %q: unmatched '{'", pattern)
		}
		close += open
		if strings.IndexByte(rest[open+1:close], '{') >= 0 {
			return nil, fmt.Errorf("host %q: nested brace groups are not supported", pattern)
		}
		inner := rest[open+1 : close]
		values, err := parseBraceGroup(inner)
		if err != nil {
			return nil, fmt.Errorf("host %q: %w", pattern, err)
		}
		segs = append(segs, segment{prefix: rest[:open], values: values})
		rest = rest[close+1:]
	}

	result := []string{""}
	count := 1
	for _, s := range segs {
		if len(s.values) > 0 {
			if maxHostExpansion/count < len(s.values) {
				return nil, fmt.Errorf("host %q: expands to more than %d machines", pattern, maxHostExpansion)
			}
			count *= len(s.values)
		}
	}
	for _, s := range segs {
		if len(s.values) == 0 {
			for i := range result {
				result[i] += s.prefix
			}
			continue
		}
		next := make([]string, 0, len(result)*len(s.values))
		for _, r := range result {
			for _, v := range s.values {
				next = append(next, r+s.prefix+v)
			}
		}
		result = next
	}
	return result, nil
}

func parseBraceGroup(inner string) ([]string, error) {
	var values []string
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, ".."); ok {
			vals, err := expandNumericRange(lo, hi, part)
			if err != nil {
				return nil, err
			}
			values = append(values, vals...)
		} else {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty brace group")
	}
	return values, nil
}

func expandNumericRange(lo, hi, raw string) ([]string, error) {
	if lo == "" || hi == "" || !isNumeric(lo) || !isNumeric(hi) {
		return nil, fmt.Errorf("invalid range %q: endpoints must be numbers", raw)
	}
	a, err := strconv.Atoi(lo)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", raw, err)
	}
	b, err := strconv.Atoi(hi)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", raw, err)
	}
	if b < a {
		return nil, fmt.Errorf("invalid range %q: %d > %d", raw, a, b)
	}
	if b-a+1 > maxHostExpansion {
		return nil, fmt.Errorf("range %q: expands to more than %d values", raw, maxHostExpansion)
	}
	pad := 0
	if len(lo) > 1 && lo[0] == '0' {
		pad = len(lo)
	}
	out := make([]string, 0, b-a+1)
	for n := a; n <= b; n++ {
		if pad > 0 {
			out = append(out, fmt.Sprintf("%0*d", pad, n))
		} else {
			out = append(out, strconv.Itoa(n))
		}
	}
	return out, nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolveMachine(mc MachineConfig, group string, cfg *Config) Machine {
	m := Machine{
		ID:    mc.Host,
		Name:  mc.Host,
		Host:  mc.Host,
		Group: group,
		Ping:  cfg.Ping,
		SSH:   cfg.SSH,
	}
	if mc.Name != "" {
		m.Name = mc.Name
	}
	if mc.Ping != nil {
		if mc.Ping.Interval.Duration > 0 {
			m.Ping.Interval = mc.Ping.Interval
		}
		if mc.Ping.Timeout.Duration > 0 {
			m.Ping.Timeout = mc.Ping.Timeout
		}
		if mc.Ping.TCPPort != 0 {
			m.Ping.TCPPort = mc.Ping.TCPPort
		}
	}
	if mc.SSH != nil {
		if mc.SSH.Interval.Duration > 0 {
			m.SSH.Interval = mc.SSH.Interval
		}
		if mc.SSH.User != "" {
			m.SSH.User = mc.SSH.User
		}
		if mc.SSH.Port != 0 {
			m.SSH.Port = mc.SSH.Port
		}
		if mc.SSH.Key != "" {
			m.SSH.Key = mc.SSH.Key
		}
		if mc.SSH.Script != "" {
			m.SSH.Script = mc.SSH.Script
		}
	}
	return m
}
