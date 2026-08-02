package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"sort"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const historyPerMachine = 5000
const historyTotalCap = 200000
const metricsPerName = 5000
const historyPruneInterval = 10 * time.Minute

type HistoryEntry struct {
	TS     int64  `json:"ts"`
	Status string `json:"status"`
	IP     string `json:"ip"`
}

type MetricEntry struct {
	TS    int64   `json:"ts"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type History struct {
	db   *sql.DB
	path string
}

func NewHistory(path string) (*History, error) {
	dsn := "file:" + path +
		"?_journal_mode=WAL" +
		"&_synchronous=NORMAL" +
		"&_busy_timeout=5000" +
		"&_foreign_keys=on" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	const schema = `
	CREATE TABLE IF NOT EXISTS history (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		machine TEXT    NOT NULL,
		ts      INTEGER NOT NULL,
		status  TEXT    NOT NULL,
		ip      TEXT    NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_history_machine_ts ON history(machine, ts);
	CREATE TABLE IF NOT EXISTS metrics (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		machine TEXT    NOT NULL,
		name    TEXT    NOT NULL,
		ts      INTEGER NOT NULL,
		value   REAL    NOT NULL,
		unit    TEXT    NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_machine_name_ts ON metrics(machine, name, ts);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &History{db: db, path: path}, nil
}

func (h *History) Close() error {
	return h.db.Close()
}

func (h *History) Size() int64 {
	if fi, err := os.Stat(h.path); err == nil {
		return fi.Size()
	}
	return 0
}

func (h *History) Stats() (rows int, firstTS int64) {
	h.db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(ts), 0) FROM history`).Scan(&rows, &firstTS)
	return
}

func (h *History) Record(machine, status, ip string) {
	_, err := h.db.Exec(
		`INSERT INTO history (machine, ts, status, ip) VALUES (?, ?, ?, ?)`,
		machine, time.Now().Unix(), status, ip,
	)
	if err != nil {
		log.Printf("history insert (%s): %v", machine, err)
	}
}

func (h *History) RecordMetric(machine, name string, ts int64, value float64, unit string) {
	_, err := h.db.Exec(
		`INSERT INTO metrics (machine, name, ts, value, unit) VALUES (?, ?, ?, ?, ?)`,
		machine, name, ts, value, unit,
	)
	if err != nil {
		log.Printf("metric insert (%s/%s): %v", machine, name, err)
	}
}

// Uptime counts the number of 'up' and 'down' history ticks for a machine.
// A machine with no history returns zeroes.
func (h *History) Uptime(machine string) (up, down int) {
	h.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END), 0)
		FROM history WHERE machine = ?`, machine).Scan(&up, &down)
	return
}

func (h *History) Query(machine string, limit int) ([]HistoryEntry, error) {
	if limit <= 0 || limit > historyPerMachine {
		limit = historyPerMachine
	}
	rows, err := h.db.Query(
		`SELECT ts, status, ip FROM history WHERE machine = ? ORDER BY id DESC LIMIT ?`,
		machine, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.TS, &e.Status, &e.IP); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (h *History) QueryMetrics(machine, name string, minTS, maxTS int64) ([]MetricEntry, error) {
	q := `SELECT ts, name, value, unit FROM metrics WHERE machine = ?`
	args := []any{machine}
	if name != "" {
		q += ` AND name = ?`
		args = append(args, name)
	}
	if minTS > 0 {
		q += ` AND ts >= ?`
		args = append(args, minTS)
	}
	if maxTS > 0 {
		q += ` AND ts < ?`
		args = append(args, maxTS)
	}
	q += ` ORDER BY id ASC LIMIT ?`
	args = append(args, metricsPerName*5)
	rows, err := h.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []MetricEntry{}
	for rows.Next() {
		var e MetricEntry
		if err := rows.Scan(&e.TS, &e.Name, &e.Value, &e.Unit); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (h *History) QueryMetricsAll(minTS, maxTS int64) ([]MetricAggregate, [2]int64, error) {
	q := `SELECT machine, ts, name, value, unit FROM metrics`
	args := []any{}
	if minTS > 0 {
		q += ` WHERE ts >= ?`
		args = append(args, minTS)
	}
	if maxTS > 0 {
		if len(args) > 0 {
			q += ` AND ts < ?`
		} else {
			q += ` WHERE ts < ?`
		}
		args = append(args, maxTS)
	}
	q += ` ORDER BY machine, name, id ASC LIMIT ?`
	args = append(args, metricsPerName*10)
	rows, err := h.db.Query(q, args...)
	if err != nil {
		return nil, [2]int64{}, err
	}
	defer rows.Close()

	type rawEntry struct {
		machine string
		entry   MetricEntry
	}
	raw := []rawEntry{}
	var firstTS, lastTS int64
	for rows.Next() {
		var e rawEntry
		if err := rows.Scan(&e.machine, &e.entry.TS, &e.entry.Name, &e.entry.Value, &e.entry.Unit); err != nil {
			return nil, [2]int64{}, err
		}
		if firstTS == 0 || e.entry.TS < firstTS {
			firstTS = e.entry.TS
		}
		if e.entry.TS > lastTS {
			lastTS = e.entry.TS
		}
		raw = append(raw, e)
	}
	if err := rows.Err(); err != nil {
		return nil, [2]int64{}, err
	}

	if minTS > 0 && firstTS < minTS {
		firstTS = minTS
	}
	if maxTS > 0 && lastTS > maxTS {
		lastTS = maxTS
	}

	type nameGroup struct {
		unit   string
		series []MetricSeries
	}
	byName := map[string]*nameGroup{}
	order := []string{}
	for _, r := range raw {
		ng, ok := byName[r.entry.Name]
		if !ok {
			ng = &nameGroup{unit: r.entry.Unit}
			byName[r.entry.Name] = ng
			order = append(order, r.entry.Name)
		}
		found := false
		for i := range ng.series {
			if ng.series[i].Machine == r.machine {
				ng.series[i].Samples = append(ng.series[i].Samples, r.entry)
				found = true
				break
			}
		}
		if !found {
			ng.series = append(ng.series, MetricSeries{Machine: r.machine, Samples: []MetricEntry{r.entry}})
		}
	}

	out := make([]MetricAggregate, 0, len(order))
	for _, name := range order {
		ng := byName[name]
		agg := MetricAggregate{Name: name, Unit: ng.unit, Series: make([]MetricSeries, 0, len(ng.series))}
		for _, s := range ng.series {
			sort.Slice(s.Samples, func(i, j int) bool { return s.Samples[i].TS < s.Samples[j].TS })
			agg.Series = append(agg.Series, s)
		}
		out = append(out, agg)
	}
	return out, [2]int64{firstTS, lastTS}, nil
}

type MetricSeries struct {
	Machine string        `json:"machine"`
	Samples []MetricEntry `json:"samples"`
}

type MetricAggregate struct {
	Name   string         `json:"name"`
	Unit   string         `json:"unit"`
	Series []MetricSeries `json:"series"`
}

func (h *History) prune() {
	res, err := h.db.Exec(
		`DELETE FROM history WHERE id NOT IN (SELECT id FROM history ORDER BY id DESC LIMIT ?)`,
		historyTotalCap,
	)
	if err != nil {
		log.Printf("history prune: %v", err)
	} else {
		n, _ := res.RowsAffected()
		if n > 0 {
			log.Printf("history pruned %d rows (kept last %d)", n, historyTotalCap)
		}
	}

	res, err = h.db.Exec(`
		DELETE FROM metrics WHERE id NOT IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (PARTITION BY machine, name ORDER BY id DESC) AS rn
				FROM metrics
			) WHERE rn <= ?
		)`, metricsPerName)
	if err != nil {
		log.Printf("metrics prune: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("metrics pruned %d rows (kept last %d per name)", n, metricsPerName)
	}
}

func (h *History) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(historyPruneInterval)
	defer ticker.Stop()
	h.prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.prune()
		}
	}
}
