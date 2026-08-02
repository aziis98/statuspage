package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const statusUp = "up"
const statusDegraded = "degraded"
const statusDown = "down"
const statusUnknown = "unknown"

const checkOK = "ok"
const checkFail = "fail"
const checkNA = "na"

const maxIPHistory = 10

// minimum interval between interactive refresh requests for the same machine
const refreshCooldown = 5 * time.Second

type sshResultState struct {
	Ok       bool      `json:"ok"`
	Result   string    `json:"result"`
	ExitCode *int      `json:"exitCode"`
	LastRun  time.Time `json:"lastRun"`
	Error    string    `json:"error"`
}

type metricState struct {
	value   float64
	unit    string
	updated time.Time
}

type machineState struct {
	mu        sync.RWMutex
	status    string
	icmp      string
	tcp       string
	lastPing  time.Time
	ip        string
	ips       []string
	sshResult *sshResultState
	metrics   map[string]metricState
}

type metricStatus struct {
	Name    string     `json:"name"`
	Value   float64    `json:"value"`
	Unit    string     `json:"unit"`
	Updated *time.Time `json:"updated"`
}

type machineStatus struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Host          string          `json:"host"`
	IP            string          `json:"ip"`
	IPs           []string        `json:"ips"`
	Status        string          `json:"status"`
	ICMP          string          `json:"icmp"`
	TCP           string          `json:"tcp"`
	LastPing      *time.Time      `json:"lastPing"`
	Uptime        *float64        `json:"uptime,omitempty"`
	SSHConfigured bool            `json:"sshConfigured"`
	SSH           *sshResultState `json:"ssh"`
	Metrics       []metricStatus  `json:"metrics"`
}

type groupStatus struct {
	Name     string          `json:"name"`
	Machines []machineStatus `json:"machines"`
}

type statsPayload struct {
	PingInterval string `json:"pingInterval"`
	PingTimeout  string `json:"pingTimeout"`
	TCPPort      int    `json:"tcpPort"`
	SSHInterval  string `json:"sshInterval"`
	SSHEnabled   bool   `json:"sshEnabled"`
	DBSize       int64  `json:"dbSize"`
	DBRows       int    `json:"dbRows"`
	DBSizeMonth  int64  `json:"dbSizeMonth"`
}

type metricRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

type statusPayload struct {
	Title              string                 `json:"title"`
	Interactive        bool                   `json:"interactive"`
	SharedMetricWindow bool                   `json:"sharedMetricWindow"`
	Stats              statsPayload           `json:"stats"`
	MetricRanges       map[string]metricRange `json:"metricRanges"`
	Groups             []groupStatus          `json:"groups"`
	Machines           []machineStatus        `json:"machines"`
}

type Monitor struct {
	cfg          *Config
	machines     []Machine
	machinesByID map[string]Machine
	interactive  bool
	ctx          context.Context
	history      *History

	icmpMu   sync.Mutex
	icmp     *icmp.PacketConn
	icmpSeq  atomic.Uint32
	icmpID   int

	statesMu sync.RWMutex
	states   map[string]*machineState

	refreshMu   sync.Mutex
	lastRefresh map[string]time.Time
}

func NewMonitor(cfg *Config, history *History) *Monitor {
	m := &Monitor{
		cfg:          cfg,
		machines:     cfg.Machines,
		machinesByID: make(map[string]Machine, len(cfg.Machines)),
		interactive:  cfg.Interactive,
		ctx:          context.Background(),
		history:      history,
		states:       make(map[string]*machineState, len(cfg.Machines)),
		lastRefresh:  make(map[string]time.Time, len(cfg.Machines)),
	}
	for _, mc := range cfg.Machines {
		m.states[mc.ID] = &machineState{status: statusUnknown, icmp: checkNA, tcp: checkNA}
		m.machinesByID[mc.ID] = mc
	}
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	}
	if err != nil {
		log.Printf("icmp unavailable (%v); falling back to TCP-only probes", err)
		m.icmp = nil
	} else {
		m.icmp = conn
		if la, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			m.icmpID = la.Port
		}
		log.Printf("icmp ok, will use it with tcp fallback")
	}
	return m
}

func (m *Monitor) Start(ctx context.Context) {
	m.ctx = ctx
	for _, mc := range m.machines {
		go m.pingLoop(ctx, mc)
		go m.sshLoop(ctx, mc)
	}
}

func (m *Monitor) Refresh(mc Machine) {
	go m.probe(m.ctx, mc)
	go m.runSSH(m.ctx, mc)
}

func (m *Monitor) pingLoop(ctx context.Context, mc Machine) {
	run := func() {
		m.probe(ctx, mc)
	}
	run()
	ticker := time.NewTicker(mc.Ping.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (m *Monitor) state(mc Machine) *machineState {
	m.statesMu.RLock()
	s := m.states[mc.ID]
	m.statesMu.RUnlock()
	return s
}

func (m *Monitor) probe(ctx context.Context, mc Machine) {
	st := m.state(mc)
	if st == nil {
		return
	}

	timeout := mc.Ping.Timeout.Duration
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	ips, err := net.DefaultResolver.LookupHost(resolveCtx, mc.Host)
	cancel()
	if err != nil {
		m.setProbeResult(st, statusDown, checkFail, checkFail, "", nil)
		m.recordHistory(mc.ID, statusDown, "")
		log.Printf("ping %s -> %s (resolve failed: %v)", mc.Host, statusDown, err)
		return
	}
	// only ping over ipv4, ignore ipv6 addresses
	var v4 []string
	for _, a := range ips {
		if net.ParseIP(a).To4() != nil {
			v4 = append(v4, a)
		}
	}
	if len(v4) == 0 {
		m.setProbeResult(st, statusDown, checkFail, checkFail, "", nil)
		m.recordHistory(mc.ID, statusDown, "")
		log.Printf("ping %s -> %s (no ipv4 addresses)", mc.Host, statusDown)
		return
	}

	ip := v4[0]
	m.recordIP(st, v4)

	icmpOK := m.icmpPing(net.ParseIP(ip), timeout)
	tcpOK := m.tcpPing(net.ParseIP(ip), mc.Ping.TCPPort, timeout)

	icmp := checkNA
	if m.icmp != nil {
		if icmpOK {
			icmp = checkOK
		} else {
			icmp = checkFail
		}
	}
	tcp := checkNA
	if tcpOK {
		tcp = checkOK
	} else {
		tcp = checkFail
	}

	status := statusDown
	if icmpOK || tcpOK {
		status = statusUp
		st.mu.RLock()
		sshBad := st.sshResult != nil && !st.sshResult.Ok
		st.mu.RUnlock()
		if (icmp == checkOK && tcp == checkFail) ||
			(icmp == checkFail && tcp == checkOK) || sshBad {
			status = statusDegraded
		}
	}

	now := time.Now()
	m.setProbeResult(st, status, icmp, tcp, ip, &now)
	m.recordHistory(mc.ID, status, ip)
	log.Printf("ping %s (%s) icmp=%s tcp=%s -> %s", mc.Host, ip, icmp, tcp, status)
}

func (m *Monitor) recordHistory(machine, status, ip string) {
	if m.history != nil {
		m.history.Record(machine, status, ip)
	}
}

func (m *Monitor) setProbeResult(st *machineState, status, icmp, tcp, ip string, last *time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.status = status
	st.icmp = icmp
	st.tcp = tcp
	if ip != "" {
		st.ip = ip
	}
	if last != nil {
		st.lastPing = *last
	}
}

func (m *Monitor) recordIP(st *machineState, ips []string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, ip := range ips {
		if ip == st.ip {
			continue
		}
		st.ips = append(st.ips, ip)
	}
	if len(st.ips) > maxIPHistory {
		st.ips = st.ips[len(st.ips)-maxIPHistory:]
	}
}

func (m *Monitor) icmpPing(ip net.IP, timeout time.Duration) bool {
	if m.icmp == nil || ip == nil {
		return false
	}
	m.icmpMu.Lock()
	defer m.icmpMu.Unlock()

	seq := int(m.icmpSeq.Add(1))
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: m.icmpID, Seq: seq, Data: []byte("lab-status")},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return false
	}
	if _, err := m.icmp.WriteTo(wb, &net.UDPAddr{IP: ip}); err != nil {
		return false
	}
	m.icmp.SetReadDeadline(time.Now().Add(timeout))
	reply := make([]byte, 1500)
	for {
		n, peer, err := m.icmp.ReadFrom(reply)
		if err != nil {
			return false
		}
		pa, ok := peer.(*net.UDPAddr)
		if !ok || !pa.IP.Equal(ip) {
			continue
		}
		rm, err := icmp.ParseMessage(1, reply[:n])
		if err != nil {
			continue
		}
		if rm.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if echo, ok := rm.Body.(*icmp.Echo); ok && echo.ID == m.icmpID && echo.Seq == seq {
			return true
		}
	}
}

func (m *Monitor) tcpPing(ip net.IP, port int, timeout time.Duration) bool {
	if ip == nil {
		return false
	}
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *Monitor) sshLoop(ctx context.Context, mc Machine) {
	if mc.SSH.Key == "" || mc.SSH.Script == "" {
		return
	}
	interval := mc.SSH.Interval.Duration
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	jitter := interval / 4
	if jitter <= 0 {
		jitter = interval
	}
	// stagger the first run within [0, interval) so machines don't all connect at once
	timer := time.NewTimer(rand.N(interval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.runSSH(ctx, mc)
			// drift each run by a random offset to keep machines out of phase
			timer.Reset(interval + rand.N(jitter))
		}
	}
}

func (m *Monitor) runSSH(ctx context.Context, mc Machine) {
	start := time.Now()
	st := m.state(mc)
	if st == nil {
		return
	}

	key, err := ssh.ParsePrivateKey([]byte(mc.SSH.Key))
	if err != nil {
		log.Printf("ssh %s: bad key: %v", mc.Host, err)
		m.setSSHError(st, fmt.Sprintf("bad ssh key: %v", err))
		return
	}

	cfg := &ssh.ClientConfig{
		User:            mc.SSH.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         mc.Ping.Timeout.Duration,
	}

	addr := net.JoinHostPort(mc.Host, strconv.Itoa(mc.SSH.Port))
	log.Printf("ssh %s: connecting to %s", mc.Host, addr)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		log.Printf("ssh %s: dial failed: %v", mc.Host, err)
		m.setSSHError(st, fmt.Sprintf("ssh dial: %v", err))
		return
	}
	defer client.Close()
	log.Printf("ssh %s: connected", mc.Host)

	session, err := client.NewSession()
	if err != nil {
		log.Printf("ssh %s: session failed: %v", mc.Host, err)
		m.setSSHError(st, fmt.Sprintf("ssh session: %v", err))
		return
	}
	defer session.Close()

	timeout := mc.SSH.Interval.Duration
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}

	log.Printf("ssh %s: running script", mc.Host)
	type cmdResult struct {
		out []byte
		err error
	}
	done := make(chan cmdResult, 1)
	go func() {
		out, err := session.CombinedOutput(mc.SSH.Script)
		done <- cmdResult{out, err}
	}()

	select {
	case r := <-done:
		var exitCode *int
		if r.err != nil {
			var ee *ssh.ExitError
			if errors.As(r.err, &ee) {
				ec := ee.ExitStatus()
				exitCode = &ec
				log.Printf("ssh %s: script done in %s, exit %d", mc.Host, time.Since(start), ec)
			} else {
				log.Printf("ssh %s: script failed: %v", mc.Host, r.err)
				m.setSSHError(st, fmt.Sprintf("ssh command: %v", r.err))
				return
			}
		} else {
			log.Printf("ssh %s: script done in %s, exit 0", mc.Host, time.Since(start))
		}
		st.mu.Lock()
		st.sshResult = &sshResultState{
			Ok:       exitCode == nil,
			Result:   string(r.out),
			ExitCode: exitCode,
			LastRun:  time.Now(),
		}
		st.mu.Unlock()
		if exitCode == nil {
			m.accumulateMetrics(st, mc.ID, string(r.out), time.Now())
		}
	case <-time.After(timeout):
		session.Close()
		log.Printf("ssh %s: timed out after %s", mc.Host, timeout)
		m.setSSHError(st, fmt.Sprintf("ssh command timed out after %s", timeout))
	case <-ctx.Done():
		log.Printf("ssh %s: aborted", mc.Host)
		return
	}
}

func (m *Monitor) setSSHError(st *machineState, msg string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sshResult = &sshResultState{
		Ok:       false,
		Error:    msg,
		LastRun:  time.Now(),
		ExitCode: nil,
	}
}

type parsedMetric struct {
	name  string
	value float64
	unit  string
}

func parseMetrics(out string) []parsedMetric {
	metrics := []parsedMetric{}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "---") {
			break
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])
		if name == "" || status != "metric" {
			continue
		}
		val := strings.TrimSpace(parts[2])
		num := 0.0
		unit := ""
		for i, r := range val {
			if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
				continue
			}
			if p, err := strconv.ParseFloat(strings.TrimSpace(val[:i]), 64); err == nil {
				num = p
				unit = strings.TrimSpace(val[i:])
			}
			break
		}
		if unit == "" && val != "" {
			if p, err := strconv.ParseFloat(val, 64); err == nil {
				num = p
			} else {
				continue
			}
		}
		metrics = append(metrics, parsedMetric{name: name, value: num, unit: unit})
	}
	return metrics
}

func (m *Monitor) accumulateMetrics(st *machineState, machineID, out string, now time.Time) {
	metrics := parseMetrics(out)
	if len(metrics) == 0 {
		return
	}
	ts := now.Unix()
	st.mu.Lock()
	if st.metrics == nil {
		st.metrics = make(map[string]metricState, len(metrics))
	}
	for _, p := range metrics {
		st.metrics[p.name] = metricState{value: p.value, unit: p.unit, updated: now}
	}
	st.mu.Unlock()
	if m.history != nil {
		for _, p := range metrics {
			m.history.RecordMetric(machineID, p.name, ts, p.value, p.unit)
		}
	}
}

func (m *Monitor) StatusPayload() statusPayload {
	var dbSize, dbSizeMonth int64
	var dbRows int
	if m.history != nil {
		dbSize = m.history.Size()
		rows, firstTS := m.history.Stats()
		dbRows = rows
		if rows > 0 && firstTS > 0 {
			elapsed := time.Now().Unix() - firstTS
			if elapsed < 1 {
				elapsed = 1
			}
			perSec := float64(dbSize) / float64(elapsed)
			dbSizeMonth = dbSize + int64(perSec*float64(30*24*3600))
		}
	}
	p := statusPayload{
		Title:              m.cfg.Title,
		Interactive:        m.interactive,
		SharedMetricWindow: m.cfg.SharedMetricWindow,
		Stats: statsPayload{
			PingInterval: m.cfg.Ping.Interval.Duration.String(),
			PingTimeout:  m.cfg.Ping.Timeout.Duration.String(),
			TCPPort:      m.cfg.Ping.TCPPort,
			SSHInterval:  m.cfg.SSH.Interval.Duration.String(),
			SSHEnabled:   m.cfg.SSH.Key != "" && m.cfg.SSH.Script != "",
			DBSize:       dbSize,
			DBRows:       dbRows,
			DBSizeMonth:  dbSizeMonth,
		},
		Groups:   []groupStatus{},
		Machines: []machineStatus{},
	}
	if len(m.cfg.Metrics) > 0 {
		p.MetricRanges = make(map[string]metricRange, len(m.cfg.Metrics))
		for name, r := range m.cfg.Metrics {
			p.MetricRanges[name] = metricRange{Min: r.Min, Max: r.Max}
		}
	}

	grouped := map[string][]machineStatus{}
	order := []string{}

	appendStatus := func(mc Machine, group string) {
		st := m.state(mc)
		ms := machineStatus{
			ID:             mc.ID,
			Name:           mc.Name,
			Host:           mc.Host,
			SSHConfigured:  mc.SSH.Key != "" && mc.SSH.Script != "",
		}
		if st != nil {
			st.mu.RLock()
			ms.Status = st.status
			ms.ICMP = st.icmp
			ms.TCP = st.tcp
			ms.IP = st.ip
			ms.IPs = append([]string{}, st.ips...)
			if !st.lastPing.IsZero() {
				t := st.lastPing
				ms.LastPing = &t
			}
			if st.sshResult != nil {
				s := *st.sshResult
				ms.SSH = &s
			}
			if len(st.metrics) > 0 {
				ms.Metrics = make([]metricStatus, 0, len(st.metrics))
				for name, mtr := range st.metrics {
					ms.Metrics = append(ms.Metrics, metricStatus{
						Name:    name,
						Value:   mtr.value,
						Unit:    mtr.unit,
						Updated: &mtr.updated,
					})
				}
			}
			st.mu.RUnlock()
		} else {
			ms.Status = statusUnknown
			ms.ICMP = checkNA
			ms.TCP = checkNA
		}
		if m.history != nil {
			up, down := m.history.Uptime(mc.ID)
			if up+down > 0 {
				u := float64(up) / float64(up+down) * 100
				ms.Uptime = &u
			}
		}
		if group == "" {
			p.Machines = append(p.Machines, ms)
			return
		}
		if _, ok := grouped[group]; !ok {
			order = append(order, group)
		}
		grouped[group] = append(grouped[group], ms)
	}

	for _, mc := range m.cfg.Machines {
		appendStatus(mc, mc.Group)
	}

	for _, name := range order {
		p.Groups = append(p.Groups, groupStatus{Name: name, Machines: grouped[name]})
	}
	return p
}

func (m *Monitor) statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.StatusPayload())
}

func (m *Monitor) historyHandler(w http.ResponseWriter, r *http.Request) {
	machine := r.PathValue("machine")
	if machine == "" {
		http.Error(w, "missing machine", http.StatusBadRequest)
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = 500
	}
	entries, err := m.history.Query(machine, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (m *Monitor) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if m.history == nil {
		http.Error(w, "no history db", http.StatusInternalServerError)
		return
	}
	machine := r.PathValue("machine")
	if machine == "" {
		http.Error(w, "missing machine", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	var minTS, maxTS int64
	if v := q.Get("min"); v != "" {
		minTS, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("max"); v != "" {
		maxTS, _ = strconv.ParseInt(v, 10, 64)
	}
	entries, err := m.history.QueryMetrics(machine, q.Get("name"), minTS, maxTS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (m *Monitor) metricsAggregateHandler(w http.ResponseWriter, r *http.Request) {
	if m.history == nil {
		http.Error(w, "no history db", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	var minTS, maxTS int64
	if v := q.Get("min"); v != "" {
		minTS, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("max"); v != "" {
		maxTS, _ = strconv.ParseInt(v, 10, 64)
	}
	metrics, window, err := m.history.QueryMetricsAll(minTS, maxTS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Window  [2]int64          `json:"window"`
		Metrics []MetricAggregate `json:"metrics"`
	}{window, metrics})
}

func (m *Monitor) refreshHandler(w http.ResponseWriter, r *http.Request) {
	if !m.interactive {
		http.Error(w, "interactive mode disabled", http.StatusForbidden)
		return
	}
	id := r.PathValue("machine")
	mc, ok := m.machinesByID[id]
	if !ok {
		http.Error(w, "machine not found", http.StatusNotFound)
		return
	}

	m.refreshMu.Lock()
	if now := time.Now(); now.Sub(m.lastRefresh[id]) < refreshCooldown {
		m.refreshMu.Unlock()
		http.Error(w, "refresh cooldown active", http.StatusTooManyRequests)
		return
	} else {
		m.lastRefresh[id] = now
	}
	m.refreshMu.Unlock()

	log.Printf("interactive refresh scheduled for %s (%s)", mc.Name, mc.Host)
	m.Refresh(mc)
	w.WriteHeader(http.StatusAccepted)
}
