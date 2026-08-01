import { useEffect, useRef, useState } from 'preact/hooks'

type Status = 'up' | 'degraded' | 'down' | 'unknown'
type Check = 'ok' | 'fail' | 'na'
type ChipState = 'ok' | 'fail' | 'na' | 'off' | 'unknown'

interface SshResult {
  ok: boolean
  result: string
  exitCode: number | null
  lastRun: string | null
  error: string | null
}

interface ScriptChip {
  name: string
  status: string
  info: string
}

interface MetricStatus {
  name: string
  value: number
  unit: string
  updated: string | null
}

interface Machine {
  id: string
  name: string
  host: string
  ip: string
  ips: string[]
  status: Status
  icmp: Check
  tcp: Check
  lastPing: string | null
  sshConfigured: boolean
  ssh: SshResult | null
  metrics: MetricStatus[]
}

interface Group {
  name: string
  machines: Machine[]
}

interface Stats {
  pingInterval: string
  pingTimeout: string
  tcpPort: number
  sshInterval: string
  sshEnabled: boolean
  dbSize: number
  dbRows: number
  dbSizeMonth: number
}

interface StatusPayload {
  title: string
  interactive: boolean
  sharedMetricWindow?: boolean
  stats: Stats
  metricRanges?: Record<string, { min?: number; max?: number }>
  groups: Group[]
  machines: Machine[]
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n
  let i = -1
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return `${v.toFixed(1)} ${units[i]}`
}

function formatTime(ts: string | null): string {
  if (!ts) return 'never'
  const d = new Date(ts)
  return isNaN(d.getTime()) ? ts : d.toLocaleString()
}

function formatMetric(m: MetricStatus): string {
  const v = Number.isInteger(m.value)
    ? String(m.value)
    : m.value.toFixed(1)
  return m.unit ? `${v} ${m.unit}` : v
}

function groupIcon(name: string): string {
  const n = name.toLowerCase()
  if (n.includes('home')) return 'mdi:home'
  if (n.includes('lab')) return 'mdi:flask'
  if (n.includes('gpu') || n.includes('compute') || n.includes('render')) return 'mdi:desktop-tower'
  if (n.includes('server') || n.includes('data') || n.includes('prod')) return 'mdi:server-network'
  if (n.includes('class') || n.includes('aula') || n.includes('room') || n.includes('student')) return 'mdi:school'
  return 'mdi:folder-outline'
}

function Dot({ status }: { status: Status }) {
  return <span class={`dot ${status}`} title={status} />
}

function Chip({
  label,
  state,
  value,
}: {
  label: string
  state?: ChipState
  value?: string
}) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const ref = useRef<HTMLSpanElement>(null)
  const hasValue = value !== undefined
  const show = () => {
    const el = ref.current
    if (!el) return
    const r = el.getBoundingClientRect()
    setPos({ x: r.left + r.width / 2, y: r.top })
  }
  const hide = () => setPos(null)
  return (
    <>
      <span
        ref={ref}
        class={`chip ${state ?? 'unknown'}`}
        title={hasValue ? undefined : `${label}: ${state}`}
        onMouseEnter={show}
        onMouseLeave={hide}
      >
        {label}
        {!hasValue && <span class="chip-dot" />}
      </span>
      {hasValue && pos && (
        <span
          class="chip-pop"
          style={{ left: `${pos.x}px`, top: `${pos.y}px` }}
        >
          {value}
        </span>
      )}
    </>
  )
}

function CheckState({ state }: { state: ChipState }) {
  return (
    <span class="check-state">
      <span class={`check-dot ${state}`} />
      {state}
    </span>
  )
}

function parseScriptChips(out: string): ScriptChip[] {
  const chips: ScriptChip[] = []
  for (const line of out.split('\n')) {
    if (line.includes('---')) break
    const i = line.indexOf(':')
    if (i <= 0) continue
    const name = line.slice(0, i).trim()
    const rest = line.slice(i + 1)
    const j = rest.indexOf(':')
    if (j < 0) continue
    const status = rest.slice(0, j).trim()
    const info = rest.slice(j + 1).trim()
    if (!name || !status || status === 'metric') continue
    chips.push({ name, status, info })
  }
  return chips
}

function scriptChipState(status: string): ChipState {
  if (status === 'on') return 'ok'
  if (status === 'off') return 'off'
  if (status === 'down') return 'fail'
  return 'unknown'
}

function MachineCard({
  m,
  onOpen,
  interactive,
  onRefresh,
}: {
  m: Machine
  onOpen: (m: Machine) => void
  interactive: boolean
  onRefresh: (m: Machine) => void
}) {
  const sshState: ChipState = !m.sshConfigured
    ? 'off'
    : !m.ssh
      ? 'unknown'
      : m.ssh.ok
        ? 'ok'
        : 'fail'
  const scriptChips = m.ssh && !m.ssh.error ? parseScriptChips(m.ssh.result) : []
  return (
    <div
      class="machine"
      data-status={m.status}
      onClick={() => onOpen(m)}
      role="button"
      tabIndex={0}
    >
      <div class="machine-head">
        <Dot status={m.status} />
        <span class="machine-name">{m.name}</span>
        {interactive && (
          <button
            class="refresh-btn"
            title="Refresh now"
            onClick={(e) => {
              e.stopPropagation()
              onRefresh(m)
            }}
          >
            <iconify-icon icon="mdi:refresh" width="15" height="15" />
          </button>
        )}
      </div>
      <div class="machine-ip">{m.ip || '—'}</div>
      <div class="chips">
        <Chip label="ping" state={m.icmp} />
        <Chip label="tcp" state={m.tcp} />
        {m.sshConfigured && <Chip label="ssh" state={sshState} />}
      </div>

      <div class="popover" onClick={(e) => e.stopPropagation()}>
        <div class="popover-head">
          <div class="popover-meta">
            <span class="popover-meta-item">
              <span class="popover-meta-key">ip</span>
              <span class="popover-meta-val">{m.ip || 'unknown'}</span>
            </span>
            <span class="popover-meta-item">
              <span class="popover-meta-key">last ping</span>
              <span class="popover-meta-val">{formatTime(m.lastPing)}</span>
            </span>
          </div>
          <span class="popover-status">
            <Dot status={m.status} />
            <span class={`badge ${m.status}`}>{m.status}</span>
          </span>
        </div>
        {scriptChips.length > 0 && (
          <table class="popover-table">
            <tbody>
              {scriptChips.map((c) => (
                <tr key={c.name}>
                  <th>{c.name}</th>
                  <td>
                    {c.status === 'unknown' ? 'unknown' : c.info || c.status}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

function Grid({
  machines,
  onOpen,
  interactive,
  onRefresh,
}: {
  machines: Machine[]
  onOpen: (m: Machine) => void
  interactive: boolean
  onRefresh: (m: Machine) => void
}) {
  return (
    <div class="grid">
      {machines.map((m) => (
        <MachineCard
          key={m.id}
          m={m}
          onOpen={onOpen}
          interactive={interactive}
          onRefresh={onRefresh}
        />
      ))}
    </div>
  )
}

interface HistoryEntry {
  ts: number
  status: Status
  ip: string
}

interface MetricEntry {
  ts: number
  name: string
  value: number
  unit: string
}

interface MetricSeries {
  machine: string
  samples: MetricEntry[]
}

interface MetricAggregate {
  name: string
  unit: string
  series: MetricSeries[]
}

interface MetricAggregatePayload {
  window: [number, number]
  metrics: MetricAggregate[]
}

function hashColor(s: string): string {
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return `hsl(${h % 360} 62% 42%)`
}

function formatTick(ts: number): string {
  const d = new Date(ts * 1000)
  return isNaN(d.getTime()) ? String(ts) : d.toLocaleString()
}

function LinePlot({
  values,
  times,
  color,
  unit,
  yMin,
  yMax,
  xMin,
  xMax,
  width = 400,
  height = 90,
  pad = 10,
}: {
  values: number[]
  times: number[]
  color: string
  unit?: string
  yMin?: number
  yMax?: number
  xMin?: number
  xMax?: number
  width?: number
  height?: number
  pad?: number
}) {
  if (values.length < 2 || times.length !== values.length) return null
  const first = values[0]!
  const last = values[values.length - 1]!
  const dataMin = Math.min(...values)
  const dataMax = Math.max(...values)
  const min = yMin !== undefined && yMin < dataMin ? yMin : dataMin
  const max = yMax !== undefined && yMax > dataMax ? yMax : dataMax
  const span = max - min || 1
  const t0 = xMin ?? times[0]!
  const t1 = xMax ?? times[times.length - 1]!
  const tspan = t1 - t0 || 1
  const xForT = (t: number) =>
    pad + ((t - t0) / tspan) * (width - pad * 2)
  const x = (i: number) => xForT(times[i]!)
  const y = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2)
  const path = values
    .map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)} ${y(v).toFixed(1)}`)
    .join(' ')

  const plotLeft = (pad / width) * 100
  const plotRight = 100 - plotLeft
  const marks = [0, 0.25, 0.5, 0.75, 1].map((f) => ({
    left: plotLeft + f * (plotRight - plotLeft),
    label: formatTickShort(t0 + f * tspan),
    transform: f === 0 ? 'translateX(0)' : f === 1 ? 'translateX(-100%)' : 'translateX(-50%)',
  }))

  const [hover, setHover] = useState<{ i: number; x: number; y: number } | null>(null)
  const boxRef = useRef<HTMLDivElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const tip = tipRef.current
    const box = boxRef.current
    if (!hover || !tip || !box) return
    const br = box.getBoundingClientRect()
    const w = tip.offsetWidth
    const minX = br.left + w / 2 + 4
    const maxX = br.right - w / 2 - 4
    tip.style.left = `${Math.min(Math.max(hover.x, minX), maxX)}px`
    tip.style.top = `${hover.y}px`
  }, [hover])

  return (
    <div class="line-plot-wrap">
      <div
        ref={boxRef}
        class="line-plot-box"
        onMouseMove={(e) => {
          const r = e.currentTarget.getBoundingClientRect()
          const vbx = ((e.clientX - r.left) / r.width) * width
          const targetT =
            t0 + ((vbx - pad) / (width - pad * 2)) * tspan
          let idx = 0
          let best = Infinity
          for (let i = 0; i < values.length; i++) {
            const d = Math.abs(times[i]! - targetT)
            if (d < best) {
              best = d
              idx = i
            }
          }
          setHover({
            i: idx,
            x: r.left + (x(idx) / width) * r.width,
            y: r.top + (y(values[idx]!) / height) * r.height,
          })
        }}
        onMouseLeave={() => setHover(null)}
      >
        <svg
          viewBox={`0 0 ${width} ${height}`}
          class="line-plot"
          preserveAspectRatio="none"
        >
          <path
            d={path}
            fill="none"
            stroke={color}
            stroke-width="1.5"
            stroke-linejoin="round"
            stroke-linecap="round"
          />
          <circle cx={x(0)} cy={y(first)} r="3" fill={color} />
          <circle cx={x(values.length - 1)} cy={y(last)} r="3" fill={color} />
        </svg>
        {hover && (
          <div
            class="plot-hover-line"
            style={{ left: `${(x(hover.i) / width) * 100}%` }}
          />
        )}
        {hover && (
          <div
            class="plot-hover-dot"
            style={{
              left: `${(x(hover.i) / width) * 100}%`,
              top: `${(y(values[hover.i]!) / height) * 100}%`,
            }}
          />
        )}
        <div class="plot-tip" ref={tipRef} style={{ visibility: hover ? 'visible' : 'hidden' }}>
          {hover
            ? `${formatTickShort(times[hover.i]!)} · ${formatMetric({ name: '', value: values[hover.i]!, unit: unit ?? '', updated: null })}`
            : ''}
        </div>
      </div>
      <div class="plot-marks">
        {marks.map((mk, i) => (
          <span key={i} class="plot-mark" style={{ left: `${mk.left}%`, transform: mk.transform }}>
            {mk.label}
          </span>
        ))}
      </div>
    </div>
  )
}

function AggregatePlot({
  series,
  color,
  xMin,
  xMax,
  yMin,
  yMax,
  width = 400,
  height = 90,
  pad = 10,
}: {
  series: MetricSeries[]
  color: string
  xMin?: number
  xMax?: number
  yMin?: number
  yMax?: number
  width?: number
  height?: number
  pad?: number
}) {
  const samples = series.flatMap((s) => s.samples)
  if (series.length === 0 || samples.length === 0) return null
  const dataMin = Math.min(...samples.map((e) => e.value))
  const dataMax = Math.max(...samples.map((e) => e.value))
  const min = yMin !== undefined && yMin < dataMin ? yMin : dataMin
  const max = yMax !== undefined && yMax > dataMax ? yMax : dataMax
  const span = max - min || 1
  const t0 = xMin ?? Math.min(...samples.map((e) => e.ts))
  const t1 = xMax ?? Math.max(...samples.map((e) => e.ts))
  const tspan = t1 - t0 || 1

  const xForT = (t: number) => pad + ((t - t0) / tspan) * (width - pad * 2)
  const y = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2)

  const paths = series
    .map((s) => {
      if (s.samples.length < 2) return null
      return s.samples
        .map((e, i) => `${i === 0 ? 'M' : 'L'}${xForT(e.ts).toFixed(1)} ${y(e.value).toFixed(1)}`)
        .join(' ')
    })
    .filter((p): p is string => p !== null)
  const lineCount = Math.max(1, paths.length)
  const opacity = 1 / lineCount

  const [hover, setHover] = useState<{
    sample: MetricEntry
    machine: string
    x: number
    y: number
  } | null>(null)
  const boxRef = useRef<HTMLDivElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const tip = tipRef.current
    const box = boxRef.current
    if (!hover || !tip || !box) return
    const br = box.getBoundingClientRect()
    const w = tip.offsetWidth
    const minX = br.left + w / 2 + 4
    const maxX = br.right - w / 2 - 4
    tip.style.left = `${Math.min(Math.max(hover.x, minX), maxX)}px`
    tip.style.top = `${hover.y}px`
  }, [hover])

  const marks = [0, 0.25, 0.5, 0.75, 1].map((f) => {
    const plotLeft = (pad / width) * 100
    const plotRight = 100 - plotLeft
    return {
      left: plotLeft + f * (plotRight - plotLeft),
      label: formatTickShort(t0 + f * tspan),
      transform: f === 0 ? 'translateX(0)' : f === 1 ? 'translateX(-100%)' : 'translateX(-50%)',
    }
  })

  return (
    <div class="line-plot-wrap">
      <div
        ref={boxRef}
        class="line-plot-box"
        onMouseMove={(e) => {
          const r = e.currentTarget.getBoundingClientRect()
          const vbx = ((e.clientX - r.left) / r.width) * width
          const targetT = t0 + ((vbx - pad) / (width - pad * 2)) * tspan
          let best: { sample: MetricEntry; machine: string } | null = null
          let bd = Infinity
          for (const s of series) {
            for (const e0 of s.samples) {
              const d = Math.abs(e0.ts - targetT)
              if (d < bd) {
                bd = d
                best = { sample: e0, machine: s.machine }
              }
            }
          }
          if (!best) return
          setHover({
            sample: best.sample,
            machine: best.machine,
            x: r.left + (xForT(best.sample.ts) / width) * r.width,
            y: r.top + (y(best.sample.value) / height) * r.height,
          })
        }}
        onMouseLeave={() => setHover(null)}
      >
        <svg
          viewBox={`0 0 ${width} ${height}`}
          class="line-plot"
          preserveAspectRatio="none"
        >
          {paths.map((d, i) => (
            <path
              key={i}
              d={d}
              fill="none"
              stroke={color}
              stroke-opacity={opacity}
              stroke-width="1.5"
              stroke-linejoin="round"
              stroke-linecap="round"
            />
          ))}
        </svg>
        {hover && (
          <div class="plot-hover-line" style={{ left: `${(xForT(hover.sample.ts) / width) * 100}%` }} />
        )}
        {hover && (
          <div
            class="plot-hover-dot"
            style={{
              left: `${(xForT(hover.sample.ts) / width) * 100}%`,
              top: `${(y(hover.sample.value) / height) * 100}%`,
            }}
          />
        )}
        <div class="plot-tip" ref={tipRef} style={{ visibility: hover ? 'visible' : 'hidden' }}>
          {hover
            ? `${formatTickShort(hover.sample.ts)} · ${hover.machine}: ${formatMetric({ name: hover.sample.name, value: hover.sample.value, unit: hover.sample.unit, updated: null })}`
            : ''}
        </div>
      </div>
      <div class="plot-marks">
        {marks.map((mk, i) => (
          <span key={i} class="plot-mark" style={{ left: `${mk.left}%`, transform: mk.transform }}>
            {mk.label}
          </span>
        ))}
      </div>
    </div>
  )
}

function formatTickShort(ts: number): string {
  const d = new Date(ts * 1000)
  if (isNaN(d.getTime())) return String(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

interface Incident {
  status: Status
  start: number
  end: number
  count: number
}

function groupIncidents(entries: HistoryEntry[]): Incident[] {
  const out: Incident[] = []
  for (const e of entries) {
    const last = out[out.length - 1]
    if (last && last.status === e.status) {
      last.end = e.ts
      last.count++
    } else {
      out.push({ status: e.status, start: e.ts, end: e.ts, count: 1 })
    }
  }
  return out
}

function StatusModal({
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

export function App() {
  const [data, setData] = useState<StatusPayload | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [modal, setModal] = useState<Machine | null>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [agg, setAgg] = useState<MetricAggregatePayload | null>(null)

  useEffect(() => {
    if (!toast) return
    const id = setTimeout(() => setToast(null), 2500)
    return () => clearTimeout(id)
  }, [toast])

  useEffect(() => {
    const poll = async () => {
      try {
        const res = await fetch('/api/status')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const d = (await res.json()) as StatusPayload
        setData(d)
        setErr(null)
        document.title = d.title || 'Status Page'
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e))
      }
    }
    poll()
    const id = setInterval(poll, 3000)
    return () => clearInterval(id)
  }, [])

  useEffect(() => {
    let cancelled = false
    const poll = async () => {
      try {
        const nowS = Math.floor(Date.now() / 1000)
        const res = await fetch(
          `/api/metrics/aggregate?min=${nowS - 86400}&max=${nowS}`
        )
        if (!res.ok) return
        const d = (await res.json()) as MetricAggregatePayload
        if (!cancelled) setAgg(d)
      } catch {
        /* keep last known aggregate */
      }
    }
    poll()
    const id = setInterval(poll, 15000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  const openMachine = (m: Machine) => {
    setModal(m)
  }

  const refreshMachine = (m: Machine) => {
    fetch(`/api/refresh/${encodeURIComponent(m.id)}`, { method: 'POST' })
      .then((r) => {
        if (r.ok) setToast(`refresh scheduled for ${m.name}`)
      })
      .catch((e) => console.error('refresh failed:', e))
  }

  return (
    <div class="page">
      <header class="header">
        <h1>{data?.title || 'Status Page'}</h1>
        {err && <span class="conn-err">offline ({err})</span>}
        <div class="legend">
          <span><span class="dot up" /> up</span>
          <span><span class="dot degraded" /> degraded</span>
          <span><span class="dot down" /> down</span>
          <span><span class="dot unknown" /> unknown</span>
        </div>
      </header>

      {data && (
        <>
          {data.groups?.map((g) => (
            <section key={g.name} class="group">
              <h2 class="group-name">
                <iconify-icon icon={groupIcon(g.name)} width="22" height="22" />
                <span>{g.name}</span>
                <span class="group-count">{g.machines.length}</span>
              </h2>
              <Grid
                machines={g.machines}
                onOpen={openMachine}
                interactive={!!data?.interactive}
                onRefresh={refreshMachine}
              />
            </section>
          ))}
          {data.machines?.length > 0 && (
            <section class="group">
              <h2 class="group-name">
                <iconify-icon icon="mdi:server" width="22" height="22" />
                <span>Machines</span>
                <span class="group-count">{data.machines.length}</span>
              </h2>
              <Grid
                machines={data.machines}
                onOpen={openMachine}
                interactive={!!data?.interactive}
                onRefresh={refreshMachine}
              />
            </section>
          )}
          {data.groups?.length === 0 && data.machines?.length === 0 && (
            <p class="empty">no machines configured</p>
          )}
        </>
      )}

      {agg && agg.metrics.length > 0 && (() => {
        const winMax = agg.window[1]
        const winMin = Math.max(winMax - 86400, agg.window[0])
        return (
          <section class="group">
            <h2 class="group-name">
              <iconify-icon icon="mdi:chart-multiple" width="22" height="22" />
              <span>Metric Aggregates</span>
              <span class="group-count">{agg.metrics.length}</span>
            </h2>
            <div class="metric-plots">
              {agg.metrics.map((m) => (
                <div key={m.name} class="metric-plot">
                  <div class="metric-plot-head">
                    <span class="metric-dot" style={{ background: hashColor(m.name) }} />
                    <strong>{m.name}</strong>
                    <span class="agg-count">
                      {m.series.length} machine{m.series.length === 1 ? '' : 's'}
                    </span>
                  </div>
                  <AggregatePlot
                    series={m.series}
                    color={hashColor(m.name)}
                    xMin={winMin}
                    xMax={winMax}
                    yMin={data?.metricRanges?.[m.name]?.min}
                    yMax={data?.metricRanges?.[m.name]?.max}
                  />
                </div>
              ))}
            </div>
          </section>
        )
      })()}

      {data && (
        <footer class="footer">
          <span>ping every {data.stats.pingInterval}</span>
          <span>ping timeout {data.stats.pingTimeout}</span>
          {data.stats.sshEnabled && (
            <span>ssh every {data.stats.sshInterval}</span>
          )}
          <span>history db {formatBytes(data.stats.dbSize)}</span>
          {data.stats.dbSizeMonth > 0 && (
            <span>
              ~{formatBytes(data.stats.dbSizeMonth)} in 1 month
            </span>
          )}
        </footer>
      )}

      {modal && (
        <StatusModal
          m={modal}
          onClose={() => setModal(null)}
          ranges={data?.metricRanges}
          sharedMetricWindow={data?.sharedMetricWindow}
        />
      )}
      {toast && <div class="toast">{toast}</div>}
    </div>
  )
}
