export function formatBytes(n?: number) {
  if (n == null) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`
}

export const formatPercent = (n?: number | null) => (n == null ? '—' : `${n.toFixed(1)} %`)

export type UsageLevel = 'none' | 'normal' | 'warning' | 'critical'

// Thresholds for resource meters (CPU, memory, disk).
export function usageLevel(percent: number | null, warnAt = 75, criticalAt = 90): UsageLevel {
  if (percent == null) return 'none'
  if (percent >= criticalAt) return 'critical'
  if (percent >= warnAt) return 'warning'
  return 'normal'
}

// Compact duration: "45s", "12m", "3h 20m", "4d 6h".
export function formatDuration(seconds: number) {
  const s = Math.max(0, Math.floor(seconds))
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d) return h ? `${d}d ${h}h` : `${d}d`
  if (h) return m ? `${h}h ${m}m` : `${h}h`
  if (m) return `${m}m`
  return `${s}s`
}

export const secondsSince = (iso: string, now = Date.now()) => (now - Date.parse(iso)) / 1000

// "3h 20m ago" / "in 7h"
export function formatRelative(iso: string, now = Date.now()) {
  const diff = secondsSince(iso, now)
  if (Math.abs(diff) < 60) return diff >= 0 ? 'just now' : 'in <1m'
  return diff >= 0 ? `${formatDuration(diff)} ago` : `in ${formatDuration(-diff)}`
}

export const formatDateTime = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
