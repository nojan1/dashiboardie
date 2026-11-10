<script setup lang="ts">
import { ref, watch } from 'vue'
import { fetchLogs, type Service } from '../api'

const props = defineProps<{ service: Service }>()
defineEmits<{ close: [] }>()

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
}

watch(() => props.service, load, { immediate: true })
</script>

<template>
  <section class="logs">
    <header>
      <h2>{{ service.name }} <span class="muted">· {{ service.source }}</span></h2>
      <button @click="load" :disabled="loading">Refresh</button>
      <button @click="$emit('close')">Close</button>
    </header>
    <p v-if="error" class="error">{{ error }}</p>
    <pre v-else>{{ loading && !lines.length ? 'Loading…' : lines.join('\n') || 'No log lines.' }}</pre>
  </section>
</template>

<style scoped>
.logs {
  margin-top: 1.5rem;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
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
  margin: 0;
  font-size: 1rem;
}
.muted { color: var(--muted); font-weight: normal; }
pre {
  margin: 0;
  padding: 0.8rem;
  max-height: 24rem;
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
</style>
