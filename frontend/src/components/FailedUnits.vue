<script setup lang="ts">
import type { FailedUnit } from '../api'

// Every unit systemd reports as failed, not only the configured ones. Shown
// only when there is something to report.
defineProps<{ units: FailedUnit[] }>()
const emit = defineEmits<{ logs: [unit: string] }>()
</script>

<template>
  <section v-if="units.length" class="failed" aria-labelledby="failed-title">
    <h2 id="failed-title">
      <span class="icon" aria-hidden="true">●</span>
      {{ units.length }} failed {{ units.length === 1 ? 'unit' : 'units' }}
    </h2>
    <ul>
      <li v-for="u in units" :key="u.unit">
        <span class="unit">{{ u.unit }}</span>
        <span v-if="u.description && u.description !== u.unit" class="desc">{{ u.description }}</span>
        <button @click="emit('logs', u.unit)">Logs</button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.failed {
  margin-bottom: 1.5rem;
  border: 1px solid var(--bad);
  border-radius: 8px;
  background: color-mix(in srgb, var(--bad) 7%, var(--surface));
  padding: 0.6rem 1rem;
}
h2 { margin: 0 0 0.4rem; font-size: 0.95rem; }
.icon { color: var(--bad); }
ul { list-style: none; margin: 0; padding: 0; }
li {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem 0.75rem;
  padding: 0.3rem 0;
  border-top: 1px solid color-mix(in srgb, var(--bad) 20%, transparent);
}
li:first-child { border-top: none; }
.unit { font: 13px var(--mono); overflow-wrap: anywhere; }
.desc { color: var(--muted); font-size: 0.85rem; flex: 1 1 8rem; min-width: 0; }
button {
  margin-left: auto;
  padding: 0.2rem 0.55rem;
  font-size: 0.85rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  cursor: pointer;
}
</style>
