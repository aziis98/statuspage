import { useEffect, useState } from 'preact/hooks'
import type { Machine, MetricAggregatePayload, StatusPayload } from './types'
import { formatBytes, groupIcon, hashColor } from './format'
import { Grid } from './components/Grid'
import { AggregatePlot } from './components/plots'
import { StatusModal } from './components/StatusModal'

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
