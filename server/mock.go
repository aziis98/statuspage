package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type mockMetric struct {
	Name, Unit string
	Base, Amp  float64
	PeriodSecs float64
	Phase      float64
	Min, Max   float64
}

type mockService struct {
	Name   string
	Status string // on | off | down
}

type mockMachine struct {
	ID, Name, Host, IP, Group, Status string
	Metrics                           []mockMetric
	Services                          []mockService
}

type mockGroup struct {
	name     string
	machines []mockMachine
}

type MockMonitor struct {
	title       string
	interactive bool
	groups      []mockGroup
	all         []mockMachine
	ranges      map[string]metricRange
}

func mockSeed(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func mockPtr(v float64) *float64 { return &v }

func NewMockMonitor() *MockMonitor {
	mm := &MockMonitor{
		title:       "statuspage",
		interactive: true,
		ranges: map[string]metricRange{
			"cpu": {Min: mockPtr(0), Max: mockPtr(100)},
		},
	}

	for i := 1; i <= 6; i++ {
		st := statusUp
		if i == 3 {
			st = statusDegraded
		}
		if i == 6 {
			st = statusDown
		}
		m := mockMachine{
			ID:     fmt.Sprintf("server-%d", i),
			Name:   fmt.Sprintf("server-%d", i),
			Host:   fmt.Sprintf("server-%d.lan", i),
			IP:     fmt.Sprintf("192.168.1.%d", 10+i),
			Group:  "Servers",
			Status: st,
			Metrics: []mockMetric{{
				Name:       "cpu",
				Unit:       "%",
				Base:       20 + float64(i*6),
				Amp:        float64(14 + i*3),
				PeriodSecs: 1200 + float64(i*360),
				Phase:      float64(i) * 0.7,
				Min:        0,
				Max:        100,
			}},
		}
		mm.all = append(mm.all, m)
	}
	mm.groups = []mockGroup{{name: "Servers", machines: mm.all}}
	return mm
}

func (mm *MockMonitor) find(id string) *mockMachine {
	for i := range mm.all {
		if mm.all[i].ID == id {
			return &mm.all[i]
		}
	}
	return nil
}

func (mm *MockMonitor) metricValue(m mockMetric, ts int64) float64 {
	t := float64(ts)
	v := m.Base + m.Amp*math.Sin(2*math.Pi*t/m.PeriodSecs+m.Phase)
	v += m.Amp * 0.35 * math.Sin(2*math.Pi*t/300+m.Phase*2.7)
	rng := rand.New(rand.NewPCG(mockSeed(m.Name), uint64(ts/45)))
	v += (rng.Float64()*2 - 1) * m.Amp * 0.55
	if m.Max > m.Min {
		v = math.Max(m.Min, math.Min(m.Max, v))
	}
	return math.Round(v*10) / 10
}

func (mm *MockMonitor) sshResult(m mockMachine, now time.Time) *sshResultState {
	var sb strings.Builder
	for _, s := range m.Services {
		info := "running"
		switch s.Status {
		case "off":
			info = "stopped"
		case "down":
			info = "not responding"
		}
		fmt.Fprintf(&sb, "%s: %s: %s\n", s.Name, s.Status, info)
	}
	for _, met := range m.Metrics {
		fmt.Fprintf(&sb, "%s: metric: %.1f %s\n", met.Name, mm.metricValue(met, now.Unix()), met.Unit)
	}
	sb.WriteString("---\n")
	sb.WriteString("uptime 6 days\nkernel 6.8.0-45-generic\n")
	return &sshResultState{Ok: true, Result: sb.String(), ExitCode: nil, LastRun: now}
}

func mockUptime(m mockMachine) *float64 {
	switch m.Status {
	case statusDown:
		u := 55.0 + float64(mockSeed(m.ID)%20)
		return &u
	case statusDegraded:
		u := 92.0 + float64(mockSeed(m.ID)%7)
		return &u
	default:
		u := 99.0 + float64(mockSeed(m.ID)%10)/10
		return &u
	}
}

func (mm *MockMonitor) machineStatus(m mockMachine, now time.Time) machineStatus {
	ms := machineStatus{
		ID: m.ID, Name: m.Name, Host: m.Host, Status: m.Status, LastPing: &now,
		IP: m.IP, IPs: []string{m.IP},
		Uptime: mockUptime(m),
	}
	switch m.Status {
	case statusDown:
		ms.ICMP, ms.TCP = checkFail, checkFail
	case statusDegraded:
		ms.ICMP, ms.TCP = checkOK, checkFail
	default:
		ms.ICMP, ms.TCP = checkOK, checkOK
	}
	if len(m.Services) > 0 || len(m.Metrics) > 0 {
		ms.SSHConfigured = true
		ms.SSH = mm.sshResult(m, now)
	}
	for _, met := range m.Metrics {
		ms.Metrics = append(ms.Metrics, metricStatus{
			Name: met.Name, Value: mm.metricValue(met, now.Unix()), Unit: met.Unit, Updated: &now,
		})
	}
	return ms
}

func (mm *MockMonitor) StatusPayload() statusPayload {
	now := time.Now()
	p := statusPayload{
		Title:              mm.title,
		Interactive:        mm.interactive,
		SharedMetricWindow: true,
		Stats: statsPayload{
			PingInterval: "15s",
			PingTimeout:  "2s",
			TCPPort:      22,
			SSHInterval:  "5m",
			SSHEnabled:   true,
			DBSize:       1400000,
			DBRows:       2300,
			DBSizeMonth:  6200000,
		},
		MetricRanges: mm.ranges,
		Groups:       []groupStatus{},
		Machines:     []machineStatus{},
	}
	for _, g := range mm.groups {
		gs := groupStatus{Name: g.name, Machines: []machineStatus{}}
		for _, m := range g.machines {
			gs.Machines = append(gs.Machines, mm.machineStatus(m, now))
		}
		p.Groups = append(p.Groups, gs)
	}
	return p
}

func (mm *MockMonitor) historyFor(m mockMachine, n int) []HistoryEntry {
	rng := rand.New(rand.NewPCG(mockSeed(m.ID), 0x9e3779b97f4a7c15))
	now := time.Now().Unix()
	const interval = int64(15)
	status := m.Status
	out := make([]HistoryEntry, 0, n)
	for i := n - 1; i >= 0; i-- {
		ts := now - int64(n-1-i)*interval
		if status != m.Status {
			if rng.Float64() < 0.22 {
				status = m.Status
			}
		} else if rng.Float64() < 0.018 {
			if rng.Float64() < 0.5 {
				status = statusDown
			} else {
				status = statusDegraded
			}
		}
		out = append(out, HistoryEntry{TS: ts, Status: status, IP: m.IP})
	}
	return out
}

func (mm *MockMonitor) metricsInWindow(m mockMachine, minTS, maxTS int64, points int) []MetricEntry {
	if points < 1 {
		points = 100
	}
	if maxTS <= minTS {
		return nil
	}
	out := []MetricEntry{}
	for i := 0; i < points; i++ {
		f := float64(i) / float64(points-1)
		ts := minTS + int64(f*float64(maxTS-minTS))
		for _, met := range m.Metrics {
			out = append(out, MetricEntry{
				TS: ts, Name: met.Name, Value: mm.metricValue(met, ts), Unit: met.Unit,
			})
		}
	}
	return out
}

func (mm *MockMonitor) statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mm.StatusPayload())
}

func (mm *MockMonitor) historyHandler(w http.ResponseWriter, r *http.Request) {
	m := mm.find(r.PathValue("machine"))
	if m == nil {
		http.Error(w, "machine not found", http.StatusNotFound)
		return
	}
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mm.historyFor(*m, limit))
}

func (mm *MockMonitor) metricsHandler(w http.ResponseWriter, r *http.Request) {
	m := mm.find(r.PathValue("machine"))
	if m == nil {
		http.Error(w, "machine not found", http.StatusNotFound)
		return
	}
	q := r.URL.Query()
	now := time.Now().Unix()
	maxTS := now
	if v := q.Get("max"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxTS = n
		}
	}
	minTS := maxTS - 86400
	if v := q.Get("min"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			minTS = n
		}
	}
	points := 100
	if v := q.Get("max_points"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			points = n
		}
	}
	name := q.Get("name")
	entries := mm.metricsInWindow(*m, minTS, maxTS, points)
	if name != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.Name == name {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (mm *MockMonitor) metricsAggregateHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now().Unix()
	maxTS := now
	if v := q.Get("max"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxTS = n
		}
	}
	minTS := maxTS - 86400
	if v := q.Get("min"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			minTS = n
		}
	}
	const points = 200
	type nameGroup struct {
		unit   string
		series []MetricSeries
	}
	byName := map[string]*nameGroup{}
	order := []string{}
	for _, m := range mm.all {
		for _, e := range mm.metricsInWindow(m, minTS, maxTS, points) {
			ng, ok := byName[e.Name]
			if !ok {
				ng = &nameGroup{unit: e.Unit}
				byName[e.Name] = ng
				order = append(order, e.Name)
			}
			if len(ng.series) == 0 || ng.series[len(ng.series)-1].Machine != m.Name {
				ng.series = append(ng.series, MetricSeries{Machine: m.Name, Samples: []MetricEntry{}})
			}
			s := &ng.series[len(ng.series)-1]
			s.Samples = append(s.Samples, e)
		}
	}
	out := make([]MetricAggregate, 0, len(order))
	for _, name := range order {
		ng := byName[name]
		out = append(out, MetricAggregate{Name: name, Unit: ng.unit, Series: ng.series})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Window  [2]int64          `json:"window"`
		Metrics []MetricAggregate `json:"metrics"`
	}{[2]int64{minTS, maxTS}, out})
}

func (mm *MockMonitor) refreshHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
}
