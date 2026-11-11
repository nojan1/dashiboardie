<script setup lang="ts">
import { computed } from 'vue'
import type { HostStats } from '../api'
import { formatBytes } from '../format'
import UsageFlag from './UsageFlag.vue'
import UsageMeter from './UsageMeter.vue'
import Sparkline from './Sparkline.vue'

const props = defineProps<{ stats: HostStats | null; error: string }>()

const pct = (used: number, total: number) => (total > 0 ? (used / total) * 100 : null)

const memPercent = computed(() => (props.stats ? pct(props.stats.memory.usedBytes, props.stats.memory.totalBytes) : null))

const filesystems = computed(() =>
  (props.stats?.filesystems ?? []).map((fs) => ({
    ...fs,
    // Same basis as `df`: used / (used + available to unprivileged users).
    percent: pct(fs.usedBytes, fs.usedBytes + fs.availBytes),
  })),
)

const fmtPct = (n: number | null) => (n == null ? '—' : `${Math.round(n)}%`)

const history = computed(() => props.stats?.history)
const cpuHistory = computed(() => (history.value?.points ?? []).map((p) => ({ t: p.t, v: p.cpu })))
const memHistory = computed(() => (history.value?.points ?? []).map((p) => ({ t: p.t, v: p.mem })))
</script>

<template>
  <section class="host" aria-label="Server resources">
    <p v-if="error" class="error">Could not load server resources: {{ error }}</p>

    <div class="tile">
      <h2>CPU</h2>
      <p class="figure">{{ fmtPct(stats?.cpu.percent ?? null) }} <UsageFlag :percent="stats?.cpu.percent ?? null" /></p>
      <UsageMeter :percent="stats?.cpu.percent ?? null" label="CPU usage" />
      <Sparkline v-if="history" :points="cpuHistory" :window-seconds="history.windowSeconds" label="CPU" />
      <p class="detail" v-if="stats">
        {{ stats.cpu.cores }} cores
        <template v-if="stats.cpu.load?.length">· load {{ stats.cpu.load.map((l) => l.toFixed(2)).join(' ') }}</template>
      </p>
    </div>

    <div class="tile">
      <h2>Memory</h2>
      <p class="figure">{{ fmtPct(memPercent) }} <UsageFlag :percent="memPercent" /></p>
      <UsageMeter :percent="memPercent" label="Memory usage" />
      <Sparkline v-if="history" :points="memHistory" :window-seconds="history.windowSeconds" label="Memory" />
      <p class="detail" v-if="stats">
        {{ formatBytes(stats.memory.usedBytes) }} of {{ formatBytes(stats.memory.totalBytes) }}
        <template v-if="stats.memory.swapTotalBytes > 0">
          · swap {{ formatBytes(stats.memory.swapUsedBytes) }} of {{ formatBytes(stats.memory.swapTotalBytes) }}
        </template>
      </p>
    </div>

    <div class="tile disks">
      <h2>Disks</h2>
      <p v-if="stats && !filesystems.length" class="detail">No filesystems found.</p>
      <ul>
        <li v-for="fs in filesystems" :key="fs.mountpoint">
          <div class="disk-head">
            <span class="mount" :title="`${fs.source} (${fs.fsType})`">{{ fs.mountpoint }}</span>
            <span v-if="fs.error" class="detail">{{ fs.error }}</span>
            <span v-else class="detail">
              <UsageFlag :percent="fs.percent" />
              {{ formatBytes(fs.usedBytes) }} of {{ formatBytes(fs.usedBytes + fs.availBytes) }} ·
              <strong>{{ fmtPct(fs.percent) }}</strong>
            </span>
          </div>
          <UsageMeter :percent="fs.error ? null : fs.percent" :label="`Disk usage of ${fs.mountpoint}`" />
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.host {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: 0.75rem;
  align-items: start;
  margin-bottom: 1.5rem;
}
.tile {
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  padding: 0.8rem 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 0;
}
.disks {
  grid-column: span 2;
}
@media (max-width: 40rem) {
  .disks { grid-column: auto; }
}
h2 {
  margin: 0;
  font-size: 0.8rem;
  text-transform: uppercase;
  color: var(--muted);
  font-weight: 600;
}
.figure {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  margin: 0;
  font-size: 2rem;
  font-weight: 600;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}
.detail {
  margin: 0;
  font-size: 0.85rem;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
}
.detail strong { color: var(--text); font-weight: 600; }
ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}
.disk-head {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.25rem;
}
.mount {
  font: 13px var(--mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.disk-head .detail { white-space: nowrap; }
.error {
  grid-column: 1 / -1;
  margin: 0;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  border: 1px solid currentColor;
  color: var(--bad);
}
</style>
