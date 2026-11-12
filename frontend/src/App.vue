<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  fetchHost,
  fetchUnits,
  listServices,
  runAction,
  serviceLogs,
  unitLogs,
  type Action,
  type HostStats,
  type LogTarget,
  type ProviderError,
  type Service,
  type UnitsOverview,
} from './api'
import { formatBytes, formatDuration, formatPercent, secondsSince } from './format'
import FailedUnits from './components/FailedUnits.vue'
import HostSummary from './components/HostSummary.vue'
import LogsModal from './components/LogsModal.vue'
import TimersTable from './components/TimersTable.vue'
import WakeOnLan from './components/WakeOnLan.vue'

const REFRESH_MS = 10_000

const services = ref<Service[]>([])
const providerErrors = ref<ProviderError[]>([])
const loadError = ref('')
const search = ref('')
const sourceFilter = ref('')
const statusFilter = ref('')
const busy = ref<Set<string>>(new Set())
const logTarget = ref<LogTarget | null>(null)
const selectedKey = ref('') // service row highlighted while its logs are open
const host = ref<HostStats | null>(null)
const hostError = ref('')
const loaded = ref(false)
const units = ref<UnitsOverview | null>(null)
const unitsError = ref('')
// Relative times ("3h ago") are computed against this; updated each refresh.
const now = ref(Date.now())

const key = (s: Service) => `${s.source}/${s.id}`

// Only http(s) links are rendered, so a crafted label or config value can't
// become a javascript: URL.
function safeUrl(url?: string) {
  if (!url) return undefined
  try {
    const u = new URL(url)
    return u.protocol === 'http:' || u.protocol === 'https:' ? u.href : undefined
  } catch {
    return undefined
  }
}

// A published port is assumed to serve http on the host the dashboard was
// opened on (location.hostname keeps IPv6 brackets).
const portUrl = (port: number) => `http://${location.hostname}:${port}/`

// "https://jellyfin.example.com/" -> "jellyfin.example.com"
const displayUrl = (url: string) => url.replace(/^https?:\/\//, '').replace(/\/$/, '')

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

async function refreshServices() {
  try {
    const res = await listServices()
    services.value = res.services
    providerErrors.value = res.errors
    loadError.value = ''
  } catch (e) {
    loadError.value = (e as Error).message
  } finally {
    loaded.value = true
  }
}

async function refreshHost() {
  try {
    host.value = await fetchHost()
    hostError.value = ''
  } catch (e) {
    hostError.value = (e as Error).message
  }
}

async function refreshUnits() {
  try {
    units.value = await fetchUnits()
    unitsError.value = ''
  } catch (e) {
    unitsError.value = (e as Error).message
  }
}

const refresh = () => {
  now.value = Date.now()
  return Promise.all([refreshServices(), refreshHost(), refreshUnits()])
}

function showServiceLogs(s: Service) {
  selectedKey.value = key(s)
  logTarget.value = serviceLogs(s)
}

function showUnitLogs(unit: string) {
  selectedKey.value = ''
  logTarget.value = unitLogs(unit)
}

function closeLogs() {
  logTarget.value = null
  selectedKey.value = ''
}

const uptime = (s: Service) => (s.startedAt ? formatDuration(secondsSince(s.startedAt, now.value)) : '')
const restartsLabel = (n: number) => `${n} ${n === 1 ? 'restart' : 'restarts'}`
const reboot = computed(() => (host.value?.reboot?.required ? host.value.reboot : null))

async function act(s: Service, action: Action) {
  const k = key(s)
  busy.value.add(k)
  try {
    await runAction(s, action)
    await refreshServices()
  } catch (e) {
    alert(`${action} ${s.name} failed: ${(e as Error).message}`)
  } finally {
    busy.value.delete(k)
  }
}

// Schedule the next refresh only after the current one finishes, so a slow
// server never has overlapping refreshes piling up. Skip while the tab is
// hidden; refresh immediately when it becomes visible again.
let timer: number | undefined
let stopped = false
let polling = false

async function poll() {
  if (polling) return // the running poll schedules the next one
  window.clearTimeout(timer)
  polling = true
  try {
    if (!document.hidden) await refresh()
  } finally {
    polling = false
  }
  if (!stopped) timer = window.setTimeout(poll, REFRESH_MS)
}

function onVisibility() {
  if (!document.hidden) poll()
}

onMounted(() => {
  poll()
  document.addEventListener('visibilitychange', onVisibility)
})
onUnmounted(() => {
  stopped = true
  window.clearTimeout(timer)
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <main>
    <h1>
      {{ host?.hostname || 'Server' }}
      <span v-if="host" class="host-uptime">up {{ formatDuration(host.uptimeSeconds) }}</span>
    </h1>

    <p v-if="reboot" class="banner reboot">
      <span class="icon" aria-hidden="true">▲</span>
      <strong>Reboot required.</strong>
      <template v-if="reboot.changed.includes('kernel') && reboot.bootedKernel !== reboot.currentKernel">
        Running {{ reboot.bootedKernel }}; the current system has {{ reboot.currentKernel }}.
      </template>
      <template v-else>The current system has a new {{ reboot.changed.join(', ') }}.</template>
    </p>

    <HostSummary :stats="host" :error="hostError" />

    <p v-if="unitsError" class="banner warn">Could not load systemd units: {{ unitsError }}</p>
    <FailedUnits :units="units?.failed ?? []" @logs="showUnitLogs" />

    <h2 class="section">Services</h2>

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
            <th>Address</th>
            <th>Status</th>
            <th>Uptime</th>
            <th class="num">CPU</th>
            <th class="num">Memory</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in filtered" :key="key(s)" :class="{ selected: selectedKey === key(s) }">
            <td><span class="tag">{{ s.source }}</span></td>
            <td>
              {{ s.name }}
              <span v-if="s.updateAvailable" class="update" title="A newer image is available in the registry">
                <span aria-hidden="true">↑</span> update
              </span>
            </td>
            <td class="address">
              <a v-if="safeUrl(s.url)" :href="safeUrl(s.url)" target="_blank" rel="noopener noreferrer">
                {{ displayUrl(s.url!) }}<span class="ext" aria-hidden="true">↗</span>
              </a>
              <span v-else-if="s.ports?.length" class="ports">
                <a
                  v-for="p in s.ports"
                  :key="p"
                  :href="portUrl(p)"
                  :aria-label="`Port ${p}`"
                  target="_blank"
                  rel="noopener noreferrer"
                >:{{ p }}<span class="ext" aria-hidden="true">↗</span></a>
              </span>
              <span v-else-if="s.address" class="ip">{{ s.address }}</span>
              <span v-else class="none">—</span>
            </td>
            <td><span class="status" :class="s.status">{{ s.status }}</span></td>
            <td class="uptime">
              <span v-if="s.startedAt" :title="`Since ${new Date(s.startedAt).toLocaleString()}`">{{ uptime(s) }}</span>
              <span v-else class="none">—</span>
              <span
                v-if="s.restarts"
                class="restarts"
                :title="`${restartsLabel(s.restarts)} by the restart policy`"
                :aria-label="restartsLabel(s.restarts)"
              >
                <span aria-hidden="true">↻</span>{{ s.restarts }}
              </span>
            </td>
            <td class="num">{{ formatPercent(s.metrics?.cpuPercent) }}</td>
            <td class="num">{{ formatBytes(s.metrics?.memoryBytes) }}</td>
            <td class="actions">
              <button :disabled="busy.has(key(s)) || s.status === 'running'" @click="act(s, 'start')">Start</button>
              <button :disabled="busy.has(key(s)) || s.status !== 'running'" @click="act(s, 'stop')">Stop</button>
              <button :disabled="busy.has(key(s))" @click="act(s, 'restart')">Restart</button>
              <button @click="showServiceLogs(s)">Logs</button>
            </td>
          </tr>
          <tr v-if="!filtered.length">
            <td colspan="8" class="empty">{{ loaded ? 'No services match.' : 'Loading…' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <TimersTable v-if="units?.timers.length" :timers="units.timers" :now="now" @logs="showUnitLogs" />

    <WakeOnLan />

    <LogsModal v-if="logTarget" :target="logTarget" @close="closeLogs" />
  </main>
</template>

<style scoped>
main {
  max-width: 1240px;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}
h1 { margin: 0 0 1rem; font-size: 1.4rem; }
.host-uptime { margin-left: 0.5rem; font-size: 0.9rem; font-weight: normal; color: var(--muted); }
.banner.reboot {
  margin-bottom: 1rem;
  border-color: var(--warn);
  background: color-mix(in srgb, var(--warn) 10%, transparent);
}
.reboot .icon { color: var(--warn); margin-right: 0.3rem; }
.section { margin: 0 0 0.75rem; font-size: 1.1rem; }
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
.address a { color: var(--accent); text-decoration: none; }
.address a:hover { text-decoration: underline; }
.address .ext { margin-left: 0.25em; font-size: 0.8em; }
.address .ip { font: 13px var(--mono); }
.address .ports { display: inline-flex; flex-wrap: wrap; gap: 0 0.6em; font: 13px var(--mono); }
.address .none { color: var(--muted); }
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
.update {
  margin-left: 0.4rem;
  padding: 0 0.45rem;
  border: 1px solid var(--accent);
  border-radius: 999px;
  font-size: 0.75rem;
}
.update span { color: var(--accent); }
.uptime { font-variant-numeric: tabular-nums; }
.uptime .none { color: var(--muted); }
.restarts { margin-left: 0.5rem; font-size: 0.8rem; }
.restarts span { color: var(--warn); }
.empty { text-align: center; color: var(--muted); padding: 1.5rem; }
</style>
