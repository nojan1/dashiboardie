<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { fetchLogs, followUrl, type LogTarget } from '../api'

// Rendered with v-if by the parent; opening is mounting, closing emits
// `close` so the parent can unmount it. Uses a native modal <dialog>, which
// provides the backdrop, focus trapping, Esc-to-close and returning focus
// to the button that opened it.
const props = defineProps<{ target: LogTarget }>()
const emit = defineEmits<{ close: [] }>()

// Live mode keeps at most this many lines in the view.
const MAX_LINES = 5000

const dialog = ref<HTMLDialogElement>()
const pre = ref<HTMLPreElement>()
const lines = ref<string[]>([])
const error = ref('')
const loading = ref(false)
const following = ref(false)
// Shown under the header while following: "Live", "Reconnecting…", ...
const streamState = ref('')

const text = computed(() =>
  loading.value && !lines.value.length ? 'Loading…' : lines.value.join('\n') || 'No log lines.',
)

const atBottom = () => {
  const el = pre.value
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

async function scrollToBottom() {
  await nextTick()
  if (pre.value) pre.value.scrollTop = pre.value.scrollHeight
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    lines.value = await fetchLogs(props.target)
  } catch (e) {
    error.value = (e as Error).message
    lines.value = []
  } finally {
    loading.value = false
  }
  // Newest lines are last; start the view there.
  scrollToBottom()
}

// --- live follow (Server-Sent Events) ---

let source: EventSource | null = null
let pending: string[] = []
let frame = 0

// Lines can arrive hundreds at a time (the backlog); render once per frame.
function flush() {
  frame = 0
  const stick = atBottom()
  const next = lines.value.concat(pending)
  pending = []
  lines.value = next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next
  if (stick) scrollToBottom()
}

function startFollow() {
  stopFollow()
  error.value = ''
  following.value = true
  streamState.value = 'Connecting…'
  const es = new EventSource(followUrl(props.target))
  source = es
  es.onopen = () => {
    // Each (re)connect starts with the backlog again, so start over.
    pending = []
    lines.value = []
    streamState.value = 'Live'
  }
  es.onmessage = (e) => {
    pending.push(e.data)
    if (!frame) frame = requestAnimationFrame(flush)
  }
  // The server sends `end` when the log source itself stopped (e.g. the
  // container exited); don't let EventSource reconnect in that case.
  es.addEventListener('end', (e) => {
    stopFollow()
    streamState.value = (e as MessageEvent).data ? `Stream ended: ${(e as MessageEvent).data}` : 'Stream ended.'
  })
  es.onerror = () => {
    if (es.readyState === EventSource.CLOSED) {
      stopFollow()
      error.value = 'Could not follow the logs.'
    } else {
      streamState.value = 'Reconnecting…'
    }
  }
}

function stopFollow() {
  source?.close()
  source = null
  following.value = false
  streamState.value = ''
}

function toggleFollow() {
  if (following.value) stopFollow()
  else startFollow()
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
onUnmounted(() => {
  stopFollow()
  cancelAnimationFrame(frame)
})
watch(
  () => props.target.path,
  () => {
    stopFollow()
    load()
  },
  { immediate: true },
)
</script>

<template>
  <dialog ref="dialog" class="logs" aria-labelledby="logs-title" @close="emit('close')" @click="onDialogClick">
    <div class="content">
      <header>
        <h2 id="logs-title">
          {{ target.title }} <span class="muted">· {{ target.subtitle }}</span>
        </h2>
        <span v-if="streamState" class="stream" :class="{ live: streamState === 'Live' }" role="status">
          {{ streamState }}
        </span>
        <button
          v-if="target.canFollow"
          :aria-pressed="following"
          :class="{ on: following }"
          title="Stream new log lines as they are written"
          @click="toggleFollow"
        >
          Follow
        </button>
        <button @click="load" :disabled="loading || following">{{ loading ? 'Loading…' : 'Refresh' }}</button>
        <button @click="close" autofocus>Close</button>
      </header>
      <p v-if="error" class="error">{{ error }}</p>
      <pre v-else ref="pre" tabindex="0">{{ text }}</pre>
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
.stream {
  font-size: 0.8rem;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 16rem;
}
.stream.live { color: var(--text); }
.stream.live::before {
  content: '';
  display: inline-block;
  width: 0.5em;
  height: 0.5em;
  margin-right: 0.35em;
  border-radius: 50%;
  background: var(--ok);
  animation: pulse 1.6s ease-in-out infinite;
}
@keyframes pulse { 50% { opacity: 0.35; } }
@media (prefers-reduced-motion: reduce) {
  .stream.live::before { animation: none; }
}
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
button.on {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 15%, var(--bg));
}
button:disabled { opacity: 0.6; cursor: default; }
@media (max-width: 36rem) {
  header { flex-wrap: wrap; }
  h2 { flex-basis: 100%; }
}
</style>
