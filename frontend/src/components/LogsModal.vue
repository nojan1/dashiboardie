<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { fetchLogs, type Service } from '../api'

// Rendered with v-if by the parent; opening is mounting, closing emits
// `close` so the parent can unmount it. Uses a native modal <dialog>, which
// provides the backdrop, focus trapping, Esc-to-close and returning focus
// to the Logs button.
const props = defineProps<{ service: Service }>()
const emit = defineEmits<{ close: [] }>()

const dialog = ref<HTMLDialogElement>()
const pre = ref<HTMLPreElement>()
const lines = ref<string[]>([])
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    lines.value = await fetchLogs(props.service)
  } catch (e) {
    error.value = (e as Error).message
    lines.value = []
  } finally {
    loading.value = false
  }
  // Newest lines are last; start the view there.
  await nextTick()
  if (pre.value) pre.value.scrollTop = pre.value.scrollHeight
}

function close() {
  dialog.value?.close() // fires the native close event -> emit
}

// Clicks on the backdrop land on the <dialog> element itself, since its
// content fills a nested wrapper.
function onDialogClick(e: MouseEvent) {
  if (e.target === dialog.value) close()
}

onMounted(() => dialog.value?.showModal())
watch(() => props.service, load, { immediate: true })
</script>

<template>
  <dialog ref="dialog" class="logs" aria-labelledby="logs-title" @close="emit('close')" @click="onDialogClick">
    <div class="content">
      <header>
        <h2 id="logs-title">
          {{ service.name }} <span class="muted">· {{ service.source }} logs</span>
        </h2>
        <button @click="load" :disabled="loading">{{ loading ? 'Loading…' : 'Refresh' }}</button>
        <button @click="close" autofocus>Close</button>
      </header>
      <p v-if="error" class="error">{{ error }}</p>
      <pre v-else ref="pre" tabindex="0">{{ loading && !lines.length ? 'Loading…' : lines.join('\n') || 'No log lines.' }}</pre>
    </div>
  </dialog>
</template>

<style scoped>
.logs {
  width: min(64rem, 100vw - 2rem);
  height: min(48rem, 100dvh - 2rem);
  max-width: none;
  max-height: none;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  box-shadow: 0 12px 32px rgb(0 0 0 / 0.3);
}
.logs::backdrop {
  background: rgb(46 52 64 / 0.6); /* nord0 */
}
.content {
  display: flex;
  flex-direction: column;
  height: 100%;
}
header {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  padding: 0.6rem 0.8rem;
  border-bottom: 1px solid var(--border);
}
h2 {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 1rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.muted { color: var(--muted); font-weight: normal; }
pre {
  flex: 1;
  margin: 0;
  padding: 0.8rem;
  overflow: auto;
  font: 13px/1.5 var(--mono);
  white-space: pre-wrap;
  word-break: break-all;
}
.error { color: var(--bad); padding: 0.8rem; margin: 0; }
button {
  padding: 0.25rem 0.7rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg);
  cursor: pointer;
}
button:disabled { opacity: 0.6; cursor: default; }
</style>
