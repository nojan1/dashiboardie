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
