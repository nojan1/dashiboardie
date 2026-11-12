<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { fetchWolHosts, wakeHost, type WolHost } from '../api'

// Tucked away at the bottom of the page in a collapsed <details>; whether it
// is open is remembered per browser. Hidden entirely when no hosts are set.
const STORAGE_KEY = 'dashiboardie.wol.open'

const hosts = ref<WolHost[]>([])
const open = ref(false)
// Per host: what happened to the last wake request.
const state = ref<Record<string, { kind: 'sending' | 'sent' | 'error'; text: string }>>({})
const timers = new Map<string, number>()

onMounted(async () => {
  try {
    open.value = localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    /* storage unavailable: start collapsed */
  }
  try {
    hosts.value = (await fetchWolHosts()).hosts
  } catch {
    hosts.value = [] // not fatal: the section just doesn't show
  }
})
onUnmounted(() => timers.forEach((t) => clearTimeout(t)))

function onToggle(e: Event) {
  open.value = (e.target as HTMLDetailsElement).open
  try {
    localStorage.setItem(STORAGE_KEY, open.value ? '1' : '0')
  } catch {
    /* ignore */
  }
}

async function wake(h: WolHost) {
  clearTimeout(timers.get(h.name))
  state.value[h.name] = { kind: 'sending', text: 'Sending…' }
  try {
    await wakeHost(h.name)
    state.value[h.name] = { kind: 'sent', text: 'Magic packet sent' }
    // The packet has no reply, so "sent" is all we can know; fade it out.
    timers.set(h.name, window.setTimeout(() => delete state.value[h.name], 6000))
  } catch (e) {
    state.value[h.name] = { kind: 'error', text: `Failed: ${(e as Error).message}` }
  }
}
</script>

<template>
  <details v-if="hosts.length" class="wol" :open="open" @toggle="onToggle">
    <summary>
      <h2>Wake on LAN</h2>
      <span class="count">{{ hosts.length }} {{ hosts.length === 1 ? 'host' : 'hosts' }}</span>
    </summary>
    <ul>
      <li v-for="h in hosts" :key="h.name">
        <span class="name">{{ h.name }}</span>
        <span class="mac" :title="`Sent to ${h.target}`">{{ h.mac }}</span>
        <span class="state" :class="state[h.name]?.kind" aria-live="polite">{{ state[h.name]?.text }}</span>
        <button :disabled="state[h.name]?.kind === 'sending'" @click="wake(h)">
          Wake<span class="sr-only"> {{ h.name }}</span>
        </button>
      </li>
    </ul>
  </details>
</template>

<style scoped>
.wol {
  margin-top: 1.5rem;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
}
summary {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  padding: 0.6rem 0.75rem;
  cursor: pointer;
  list-style: none;
  border-radius: 8px;
}
summary::-webkit-details-marker { display: none; }
summary::before {
  content: '▸';
  color: var(--muted);
  transition: transform 0.15s;
}
.wol[open] summary::before { transform: rotate(90deg); }
summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
h2 { margin: 0; font-size: 1rem; }
.count { color: var(--muted); font-size: 0.85rem; }
ul { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--border); }
li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem 0.9rem;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--border);
}
li:last-child { border-bottom: none; }
.name { min-width: 8rem; }
.mac { font: 13px var(--mono); color: var(--muted); }
.state { margin-left: auto; font-size: 0.85rem; color: var(--muted); }
.state.sent { color: var(--text); }
.state.sent::before { content: '✓ '; color: var(--ok); }
.state.error { color: var(--bad); }
button {
  padding: 0.2rem 0.7rem;
  font-size: 0.85rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--surface);
  cursor: pointer;
}
button:disabled { opacity: 0.45; cursor: default; }
li:has(.state:empty) button { margin-left: auto; }
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
