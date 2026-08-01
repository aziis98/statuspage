import { useEffect, useRef, useState } from 'preact/hooks'
import type { HistoryEntry, Machine, MetricEntry } from '../types'
import { formatMetric, formatTick, formatTickShort, formatTime, hashColor } from '../format'
import { groupIncidents, parseScriptChips, scriptChipState } from '../parse'
import { CheckState, Dot } from './ui'
import { LinePlot } from './plots'

export function StatusModal({
  m,
  onClose,
  ranges,
  sharedMetricWindow,
}: {
  m: Machine
  onClose: () => void
  ranges?: Record<string, { min?: number; max?: number }>
  sharedMetricWindow?: boolean
}) {
  const [hist, setHist] = useState<HistoryEntry[] | null>(null)
  const [metricEntries, setMetricEntries] = useState<MetricEntry[] | null>(null)
  const ticksRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = ticksRef.current
    if (el) el.scrollLeft = el.scrollWidth
  }, [hist])

  const scriptChips = m.ssh && !m.ssh.error ? parseScriptChips(m.ssh.result) : []
  const scriptText = (() => {
    if (!m.ssh) return ''
    const out = m.ssh.error || m.ssh.result || '(no output)'
    const sep = out.indexOf('---')
    return sep === -1 ? out : out.slice(sep + 3).trim()
  })()

  useEffect(() => {
    let cancelled = false
    setHist(null)
    fetch(`/api/history/${encodeURIComponent(m.id)}?limit=500`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((entries: HistoryEntry[]) => {
        if (!cancelled) setHist(entries)
      })
      .catch((e) => {
        if (!cancelled) setHist([])
        console.error('history fetch failed:', e)
      })
    return () => {
      cancelled = true
    }
  }, [m.id])

  const metricNames = (m.metrics ?? []).map((mt) => mt.name)

  useEffect(() => {
    let cancelled = false
    setMetricEntries(null)
    const nowS = Math.floor(Date.now() / 1000)
    const min = nowS - 86400
    const shared = sharedMetricWindow ? '&shared=1' : ''
    Promise.all(
      metricNames.map((name) =>
        fetch(
          `/api/metrics/${encodeURIComponent(m.id)}?name=${encodeURIComponent(name)}&min=${min}&max=${nowS}&max_points=100${shared}`
        ).then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      )
    )
      .then((lists: MetricEntry[][]) => {
        if (cancelled) return
        const all: MetricEntry[] = []
        for (const l of lists) all.push(...l)
        setMetricEntries(all)
      })
      .catch((e) => {
        if (!cancelled) setMetricEntries([])
        console.error('metrics fetch failed:', e)
      })
    return () => {
      cancelled = true
    }
    }, [m.id, metricNames.join(','), sharedMetricWindow])

  const metricPlots = (() => {
    if (!metricEntries) return null
    const byName = new Map<string, MetricEntry[]>()
    for (const e of metricEntries) {
      if (!byName.has(e.name)) byName.set(e.name, [])
      byName.get(e.name)!.push(e)
    }
    const winMax = Math.floor(Date.now() / 1000)
    let winMin = winMax - 86400
    if (sharedMetricWindow && metricEntries.length > 0) {
      const earliest = Math.min(...metricEntries.map((e) => e.ts))
      winMin = Math.max(winMin, earliest)
    }
    const plots: {
      name: string
      unit: string
      values: number[]
      times: number[]
      yMin?: number
      yMax?: number
      xMin?: number
      xMax?: number
    }[] = []
    for (const [name, entries] of byName) {
      const r = ranges?.[name]
      plots.push({
        name,
        unit: entries[0]?.unit ?? '',
        values: entries.map((e) => e.value),
        times: entries.map((e) => e.ts),
        yMin: r?.min,
        yMax: r?.max,
        xMin: sharedMetricWindow ? winMin : undefined,
        xMax: sharedMetricWindow ? winMax : undefined,
      })
    }
    plots.sort((a, b) => a.name.localeCompare(b.name))
    return plots
  })()

  const ticks = hist ? [...hist].reverse() : []
  const counts = { up: 0, degraded: 0, down: 0, unknown: 0 }
  hist?.forEach((e) => {
    if (counts[e.status] !== undefined) counts[e.status]++
  })

  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal" onClick={(e) => e.stopPropagation()}>
        <div class="modal-head">
          <Dot status={m.status} />
          <h2>{m.name}</h2>
          <button class="modal-close" onClick={onClose} aria-label="close">
            ×
          </button>
        </div>
        <div class="modal-grid">
          <div class="modal-cell">
            <span class="modal-cell-label">host</span>
            <span class="modal-cell-value">{m.host}</span>
          </div>
          <div class="modal-cell">
            <span class="modal-cell-label">status</span>
            <span class="modal-cell-value">
              <span class={`badge ${m.status}`}>{m.status}</span>
            </span>
          </div>
          <div class="modal-cell">
            <span class="modal-cell-label">ping</span>
            <span class="modal-cell-value">
              <CheckState state={m.icmp} />
            </span>
          </div>
          <div class="modal-cell">
            <span class="modal-cell-label">tcp</span>
            <span class="modal-cell-value">
              <CheckState state={m.tcp} />
            </span>
          </div>
          {m.sshConfigured && (
            <div class="modal-cell">
              <span class="modal-cell-label">ssh</span>
              <span class="modal-cell-value">
                <CheckState state={!m.ssh ? 'unknown' : m.ssh.ok ? 'ok' : 'fail'} />
              </span>
            </div>
          )}
          {scriptChips.map((c) => (
            <div class="modal-cell" key={c.name}>
              <span class="modal-cell-label">{c.name}</span>
              <span class="modal-cell-value">
                <span class="check-state">
                  <span class={`check-dot ${scriptChipState(c.status)}`} />
                  {c.status === 'unknown' ? 'unknown' : c.info || c.status}
                </span>
              </span>
            </div>
          ))}
          {(m.metrics ?? []).map((mt) => (
            <div class="modal-cell" key={mt.name}>
              <span class="modal-cell-label" title={mt.updated ? `updated ${formatTime(mt.updated)}` : undefined}>
                {mt.name}
              </span>
              <span class="modal-cell-value metric-value">
                {formatMetric(mt)}
              </span>
            </div>
          ))}
          <div class="modal-cell">
            <span class="modal-cell-label">current ip</span>
            <span class="modal-cell-value mono">{m.ip || 'unknown'}</span>
          </div>
          <div class="modal-cell">
            <span class="modal-cell-label">ip history</span>
            <span class="modal-cell-value mono">
              {(() => {
                const seen = new Set<string>()
                const last: string[] = []
                if (m.ips) {
                  for (let i = m.ips.length - 1; i >= 0 && seen.size < 5; i--) {
                    const ip = m.ips[i]
                    if (ip && !seen.has(ip)) {
                      seen.add(ip)
                      last.unshift(ip)
                    }
                  }
                }
                return last.length ? last.join(' · ') : 'none recorded'
              })()}
            </span>
          </div>
          <div class="modal-cell">
            <span class="modal-cell-label">last ping</span>
            <span class="modal-cell-value mono">{formatTime(m.lastPing)}</span>
          </div>
          {m.ssh && (
            <>
              <div class="modal-cell">
                <span class="modal-cell-label">script run</span>
                <span class="modal-cell-value mono">
                  {formatTime(m.ssh.lastRun)}
                  {m.ssh.exitCode !== null && ` · exit ${m.ssh.exitCode}`}
                </span>
              </div>
              <div class="modal-cell full">
                <span class="modal-cell-label">script output</span>
                <pre class="script-out tall">{scriptText}</pre>
              </div>
            </>
          )}
        </div>

        <div class="hist">
          <div class="hist-head">
            <strong>tick history</strong>
            {hist === null && <span class="muted">loading…</span>}
            {hist && hist.length === 0 && <span class="muted">no history yet</span>}
            {hist && hist.length > 0 && (
              <span class="hist-counts">
                <span><span class="dot up" /> {counts.up}</span>
                <span><span class="dot degraded" /> {counts.degraded}</span>
                <span><span class="dot down" /> {counts.down}</span>
              </span>
            )}
          </div>
          {hist && hist.length > 0 && (
            <div class="ticks" ref={ticksRef}>
              {ticks.map((e, i) => {
                const step = Math.max(1, Math.round(ticks.length / 4))
                const mark =
                  i === 0 ||
                  i === ticks.length - 1 ||
                  i % step === 0
                return (
                  <span key={i} class="tick-cell">
                    <span
                      class={`tick ${e.status}`}
                      title={`${formatTick(e.ts)} · ${e.status}${e.ip ? ` · ${e.ip}` : ''}`}
                    />
                    {mark && (
                      <span class="tick-mark">{formatTickShort(e.ts)}</span>
                    )}
                  </span>
                )
              })}
            </div>
          )}
          {hist && hist.length > 0 && (
            <div class="incidents">
              <div class="incidents-title">incidents</div>
              {groupIncidents(ticks)
                .slice(-30)
                .reverse()
                .map((inc, i) => (
                  <div key={i} class="incident-row">
                    <span class={`badge ${inc.status}`}>{inc.status}</span>
                    <span>
                      {formatTickShort(inc.start)}
                      {inc.count > 1
                        ? ` → ${formatTickShort(inc.end)}`
                        : ''}
                    </span>
                    <span class="muted">
                      {inc.count > 1 ? `${inc.count} ticks` : ''}
                    </span>
                  </div>
                ))}
            </div>
          )}
          {metricPlots && metricPlots.length > 0 && (
            <div class="metric-plots">
              <div class="incidents-title">metrics</div>
              {metricPlots.map((p) => (
                <div key={p.name} class="metric-plot">
                  <div class="metric-plot-head">
                    <span
                      class="metric-dot"
                      style={{ background: hashColor(p.name) }}
                    />
                    <strong>{p.name}</strong>
                    <span
                      class={`metric-latest${p.yMin !== undefined && p.values[p.values.length - 1]! < p.yMin || p.yMax !== undefined && p.values[p.values.length - 1]! > p.yMax ? ' out-of-range' : ''}`}
                    >
                      {formatMetric({ name: p.name, value: p.values[p.values.length - 1]!, unit: p.unit, updated: null })}
                    </span>
                  </div>
                  <LinePlot
                    values={p.values}
                    times={p.times}
                    color={hashColor(p.name)}
                    unit={p.unit}
                    yMin={p.yMin}
                    yMax={p.yMax}
                    xMin={p.xMin}
                    xMax={p.xMax}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
