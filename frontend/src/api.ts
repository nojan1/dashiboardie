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

export const fetchLogs = (s: Service, lines = 200) =>
  request<{ lines: string[] }>(`${servicePath(s)}/logs?lines=${lines}`).then((r) => r.lines)

// Mirrors backend/internal/host.
export interface HostStats {
  hostname: string
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
