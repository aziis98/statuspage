import type { MetricStatus } from './types'

export function formatBytes(n: number): string {
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

export function formatTime(ts: string | null): string {
  if (!ts) return 'never'
  const d = new Date(ts)
  return isNaN(d.getTime()) ? ts : d.toLocaleString()
}

export function formatMetric(m: MetricStatus): string {
  const v = Number.isInteger(m.value)
    ? String(m.value)
    : m.value.toFixed(1)
  return m.unit ? `${v} ${m.unit}` : v
}

export function formatTick(ts: number): string {
  const d = new Date(ts * 1000)
  return isNaN(d.getTime()) ? String(ts) : d.toLocaleString()
}

export function formatTickShort(ts: number): string {
  const d = new Date(ts * 1000)
  if (isNaN(d.getTime())) return String(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function groupIcon(name: string): string {
  const n = name.toLowerCase()
  if (n.includes('home')) return 'mdi:home'
  if (n.includes('lab')) return 'mdi:flask'
  if (n.includes('gpu') || n.includes('compute') || n.includes('render')) return 'mdi:desktop-tower'
  if (n.includes('server') || n.includes('data') || n.includes('prod')) return 'mdi:server-network'
  if (n.includes('class') || n.includes('aula') || n.includes('room') || n.includes('student')) return 'mdi:school'
  return 'mdi:folder-outline'
}

export function hashColor(s: string): string {
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0
  return `hsl(${h % 360} 62% 42%)`
}
