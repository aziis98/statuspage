import { useEffect, useRef, useState } from 'preact/hooks'
import type { MetricEntry, MetricSeries } from '../types'
import { formatMetric, formatTickShort } from '../format'

export function LinePlot({
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
          const vby = ((e.clientY - r.top) / r.height) * height
          let idx = 0
          let best = Infinity
          for (let i = 0; i < values.length; i++) {
            const dx = x(i) - vbx
            const dy = y(values[i]!) - vby
            const d = dx * dx + dy * dy
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

export function AggregatePlot({
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
  // gentler 1/sqrt(n) falloff: isolated lines stay visible while dense clusters
  // still accumulate to a solid mass
  const opacity = Math.max(0.25, 1 / Math.sqrt(lineCount))

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
          const vby = ((e.clientY - r.top) / r.height) * height
          let best: { sample: MetricEntry; machine: string } | null = null
          let bd = Infinity
          for (const s of series) {
            for (const e0 of s.samples) {
              const dx = xForT(e0.ts) - vbx
              const dy = y(e0.value) - vby
              const d = dx * dx + dy * dy
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
          class="line-plot agg-plot"
          preserveAspectRatio="none"
        >
          {paths.map((d, i) => (
            <path
              key={i}
              class="agg-line"
              d={d}
              fill="none"
              stroke={color}
              stroke-opacity={opacity}
              stroke-width="1"
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
