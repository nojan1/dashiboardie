<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { listServices, runAction, type Action, type ProviderError, type Service } from './api'
import LogsPanel from './components/LogsPanel.vue'

const REFRESH_MS = 10_000

const services = ref<Service[]>([])
const providerErrors = ref<ProviderError[]>([])
const loadError = ref('')
const search = ref('')
const sourceFilter = ref('')
const statusFilter = ref('')
const busy = ref<Set<string>>(new Set())
const selected = ref<Service | null>(null)

const key = (s: Service) => `${s.source}/${s.id}`

const sources = computed(() => [...new Set(services.value.map((s) => s.source))].sort())
const statuses = computed(() => [...new Set(services.value.map((s) => s.status))].sort())

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return services.value.filter(
    (s) =>
      (!sourceFilter.value || s.source === sourceFilter.value) &&
      (!statusFilter.value || s.status === statusFilter.value) &&
      (!q || s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q)),
  )
})

async function refresh() {
  try {
    const res = await listServices()
    services.value = res.services
    providerErrors.value = res.errors
    loadError.value = ''
  } catch (e) {
    loadError.value = (e as Error).message
  }
}

async function act(s: Service, action: Action) {
  const k = key(s)
  busy.value.add(k)
  try {
    await runAction(s, action)
    await refresh()
  } catch (e) {
    alert(`${action} ${s.name} failed: ${(e as Error).message}`)
  } finally {
    busy.value.delete(k)
  }
}

function formatBytes(n?: number) {
  if (n == null) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i ? 1 : 0)} ${units[i]}`
}

const formatCpu = (n?: number) => (n == null ? '—' : `${n.toFixed(1)} %`)

let timer: number | undefined
onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, REFRESH_MS)
})
onUnmounted(() => window.clearInterval(timer))
</script>

<template>
  <main>
    <h1>Services</h1>

    <div class="toolbar">
      <input v-model="search" type="search" placeholder="Search by name…" />
      <select v-model="sourceFilter">
        <option value="">All sources</option>
        <option v-for="s in sources" :key="s" :value="s">{{ s }}</option>
      </select>
      <select v-model="statusFilter">
        <option value="">All statuses</option>
        <option v-for="s in statuses" :key="s" :value="s">{{ s }}</option>
      </select>
      <button @click="refresh">Refresh</button>
    </div>

    <p v-if="loadError" class="banner bad">Could not load services: {{ loadError }}</p>
    <p v-for="e in providerErrors" :key="e.source" class="banner warn">
      {{ e.source }}: {{ e.error }}
    </p>

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Source</th>
            <th>Name</th>
            <th>Status</th>
            <th class="num">CPU</th>
            <th class="num">Memory</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in filtered" :key="key(s)" :class="{ selected: selected && key(selected) === key(s) }">
            <td><span class="tag">{{ s.source }}</span></td>
            <td>{{ s.name }}</td>
            <td><span class="status" :class="s.status">{{ s.status }}</span></td>
            <td class="num">{{ formatCpu(s.metrics?.cpuPercent) }}</td>
            <td class="num">{{ formatBytes(s.metrics?.memoryBytes) }}</td>
            <td class="actions">
              <button :disabled="busy.has(key(s)) || s.status === 'running'" @click="act(s, 'start')">Start</button>
              <button :disabled="busy.has(key(s)) || s.status !== 'running'" @click="act(s, 'stop')">Stop</button>
              <button :disabled="busy.has(key(s))" @click="act(s, 'restart')">Restart</button>
              <button @click="selected = s">Logs</button>
            </td>
          </tr>
          <tr v-if="!filtered.length">
            <td colspan="6" class="empty">No services match.</td>
          </tr>
        </tbody>
      </table>
    </div>

    <LogsPanel v-if="selected" :service="selected" @close="selected = null" />
  </main>
</template>

<style scoped>
main {
  max-width: 1100px;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}
h1 { margin: 0 0 1rem; font-size: 1.4rem; }
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1rem;
}
.toolbar input { flex: 1 1 14rem; }
input, select, button {
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
}
button { cursor: pointer; }
button:disabled { opacity: 0.45; cursor: default; }
.banner {
  margin: 0 0 0.5rem;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  border: 1px solid currentColor;
}
.banner.bad { color: var(--bad); }
.banner.warn { color: var(--warn); }
.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
}
table { width: 100%; border-collapse: collapse; }
th, td {
  padding: 0.5rem 0.75rem;
  text-align: left;
  border-bottom: 1px solid var(--border);
  white-space: nowrap;
}
tbody tr:last-child td { border-bottom: none; }
th { font-size: 0.8rem; text-transform: uppercase; color: var(--muted); font-weight: 600; }
tr.selected td { background: color-mix(in srgb, var(--accent) 8%, transparent); }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.tag { font: 12px var(--mono); color: var(--muted); }
.status::before {
  content: '';
  display: inline-block;
  width: 0.55em;
  height: 0.55em;
  margin-right: 0.4em;
  border-radius: 50%;
  background: var(--muted);
}
.status.running::before { background: var(--ok); }
.status.failed::before { background: var(--bad); }
.actions { display: flex; gap: 0.3rem; justify-content: flex-end; }
.actions button { padding: 0.2rem 0.55rem; font-size: 0.85rem; }
.empty { text-align: center; color: var(--muted); padding: 1.5rem; }
</style>
