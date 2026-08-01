package main

import (
	"fmt"
	"os"
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
			cfg.Machines = append(cfg.Machines, resolveMachine(mc, g.Name, cfg))
		}
	}
	for _, mc := range raw.Machines {
		cfg.Machines = append(cfg.Machines, resolveMachine(mc, "", cfg))
	}
	return cfg, nil
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
