import type { ChipState, HistoryEntry, Incident, ScriptChip } from './types'

export function parseScriptChips(out: string): ScriptChip[] {
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

export function scriptChipState(status: string): ChipState {
  if (status === 'on') return 'ok'
  if (status === 'off') return 'off'
  if (status === 'down') return 'fail'
  return 'unknown'
}

export function groupIncidents(entries: HistoryEntry[]): Incident[] {
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
