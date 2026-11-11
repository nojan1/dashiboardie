// Mirrors the Go model in backend/internal/service/service.go.
export type Source = 'podman' | 'quadlet' | 'incus' | 'nixos' | 'systemd'
export type Status = 'running' | 'stopped' | 'failed' | 'unknown'
export type Action = 'start' | 'stop' | 'restart'

export interface Metrics {
  cpuPercent?: number
  memoryBytes?: number
}

export interface Service {
  id: string
  source: Source
  name: string
  status: Status
  metrics?: Metrics
  unit?: string
  url?: string
  address?: string
  startedAt?: string // RFC 3339; only while running
  restarts?: number
  updateAvailable?: boolean
}

export interface ProviderError {
  source: Source
  error: string
}

export interface ServiceList {
  services: Service[]
  errors: ProviderError[]
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}

const servicePath = (s: Service) =>
  `/api/services/${encodeURIComponent(s.source)}/${encodeURIComponent(s.id)}`

export const listServices = () => request<ServiceList>('/api/services')

export const runAction = (s: Service, action: Action) =>
  request<void>(`${servicePath(s)}/${action}`, { method: 'POST' })

// Something whose logs can be shown: a service or a host-level unit.
// `path` is the logs endpoint; `${path}/follow` streams it (SSE).
export interface LogTarget {
  title: string
  subtitle: string
  path: string
  // incus console logs can't be followed.
  canFollow: boolean
}

export const serviceLogs = (s: Service): LogTarget => ({
  title: s.name,
  subtitle: `${s.source} logs`,
  path: `${servicePath(s)}/logs`,
  canFollow: s.source !== 'incus',
})

export const unitLogs = (unit: string): LogTarget => ({
  title: unit,
  subtitle: 'journal',
  path: `/api/units/${encodeURIComponent(unit)}/logs`,
  canFollow: true,
})

export const fetchLogs = (t: LogTarget, lines = 200) =>
  request<{ lines: string[] }>(`${t.path}?lines=${lines}`).then((r) => r.lines)

export const followUrl = (t: LogTarget, lines = 200) => `${t.path}/follow?lines=${lines}`

// Mirrors backend/internal/host.
export interface HistoryPoint {
  t: number // unix seconds
  cpu: number | null
  mem: number
}

export interface HostStats {
  hostname: string
  uptimeSeconds: number
  reboot?: {
    required: boolean
    changed: string[]
    bootedKernel: string
    currentKernel: string
  }
  history?: { intervalSeconds: number; windowSeconds: number; points: HistoryPoint[] }
  cpu: { percent: number | null; cores: number; load: number[] | null }
  memory: { totalBytes: number; usedBytes: number; swapTotalBytes: number; swapUsedBytes: number }
  filesystems: {
    mountpoint: string
    source: string
    fsType: string
    totalBytes: number
    usedBytes: number
    availBytes: number
    error?: string
  }[]
}

export const fetchHost = () => request<HostStats>('/api/host')

// Mirrors backend/internal/units.
export interface FailedUnit {
  unit: string
  description: string
}

export interface Timer {
  name: string
  unit: string
  description?: string
  service?: string
  found: boolean
  active: boolean
  lastRun?: string
  nextRun?: string
  running: boolean
  result?: string
  exitStatus?: number
}

export interface UnitsOverview {
  failed: FailedUnit[]
  timers: Timer[]
}

export const fetchUnits = () => request<UnitsOverview>('/api/units')
