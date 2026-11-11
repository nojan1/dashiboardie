<script setup lang="ts">
import type { Timer } from '../api'
import { formatDateTime, formatRelative } from '../format'

defineProps<{ timers: Timer[]; now: number }>()
const emit = defineEmits<{ logs: [unit: string] }>()

// A short, colour-independent summary of the last run.
function result(t: Timer): { text: string; level: 'ok' | 'bad' | 'none' } {
  if (t.running) return { text: 'running', level: 'none' }
  if (!t.result) return { text: 'never ran', level: 'none' }
  if (t.result === 'success') return { text: 'succeeded', level: 'ok' }
  const why = t.result === 'exit-code' && t.exitStatus != null ? `exit ${t.exitStatus}` : t.result
  return { text: `failed (${why})`, level: 'bad' }
}
</script>

<template>
  <section aria-labelledby="timers-title">
    <h2 id="timers-title" class="section">Timers</h2>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Last run</th>
            <th>Result</th>
            <th>Next run</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in timers" :key="t.unit">
            <td>
              {{ t.name }}
              <span v-if="t.description" class="desc">{{ t.description }}</span>
            </td>
            <template v-if="!t.found">
              <td colspan="3" class="muted">No such timer ({{ t.unit }})</td>
            </template>
            <template v-else>
              <td :title="t.lastRun ? formatDateTime(t.lastRun) : undefined">
                {{ t.lastRun ? formatRelative(t.lastRun, now) : '—' }}
              </td>
              <td>
                <span class="result" :class="result(t).level">{{ result(t).text }}</span>
              </td>
              <td :title="t.nextRun ? formatDateTime(t.nextRun) : undefined">
                <template v-if="!t.active">stopped</template>
                <template v-else>{{ t.nextRun ? formatRelative(t.nextRun, now) : '—' }}</template>
              </td>
            </template>
            <td class="actions">
              <button v-if="t.found" @click="emit('logs', t.service || t.unit)">Logs</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.section { margin: 1.5rem 0 0.75rem; font-size: 1.1rem; }
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
.desc { margin-left: 0.4rem; color: var(--muted); font-size: 0.85rem; }
.muted { color: var(--muted); }
.result::before {
  content: '';
  display: inline-block;
  width: 0.55em;
  height: 0.55em;
  margin-right: 0.4em;
  border-radius: 50%;
  background: var(--muted);
}
.result.ok::before { background: var(--ok); }
.result.bad::before { background: var(--bad); }
.actions { text-align: right; }
button {
  padding: 0.2rem 0.55rem;
  font-size: 0.85rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  cursor: pointer;
}
</style>
