export type Status = 'up' | 'degraded' | 'down' | 'unknown'
export type Check = 'ok' | 'fail' | 'na'
export type ChipState = 'ok' | 'fail' | 'na' | 'off' | 'unknown'

export interface SshResult {
  ok: boolean
  result: string
  exitCode: number | null
  lastRun: string | null
  error: string | null
}

export interface ScriptChip {
  name: string
  status: string
  info: string
}

export interface MetricStatus {
  name: string
  value: number
  unit: string
  updated: string | null
}

export interface Machine {
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

export interface Group {
  name: string
  machines: Machine[]
}

export interface Stats {
  pingInterval: string
  pingTimeout: string
  tcpPort: number
  sshInterval: string
  sshEnabled: boolean
  dbSize: number
  dbRows: number
  dbSizeMonth: number
}

export interface StatusPayload {
  title: string
  interactive: boolean
  sharedMetricWindow?: boolean
  stats: Stats
  metricRanges?: Record<string, { min?: number; max?: number }>
  groups: Group[]
  machines: Machine[]
}

export interface HistoryEntry {
  ts: number
  status: Status
  ip: string
}

export interface MetricEntry {
  ts: number
  name: string
  value: number
  unit: string
}

export interface MetricSeries {
  machine: string
  samples: MetricEntry[]
}

export interface MetricAggregate {
  name: string
  unit: string
  series: MetricSeries[]
}

export interface MetricAggregatePayload {
  window: [number, number]
  metrics: MetricAggregate[]
}

export interface Incident {
  status: Status
  start: number
  end: number
  count: number
}
