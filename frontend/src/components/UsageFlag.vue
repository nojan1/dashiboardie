<script setup lang="ts">
import { computed } from 'vue'
import { usageLevel } from '../format'

const props = defineProps<{ percent: number | null }>()
const level = computed(() => usageLevel(props.percent))
</script>

<template>
  <span v-if="level === 'warning'" class="flag"><span class="icon warning" aria-hidden="true">▲</span> High</span>
  <span v-else-if="level === 'critical'" class="flag"><span class="icon critical" aria-hidden="true">●</span> Critical</span>
</template>

<style scoped>
.flag {
  font-size: 0.75rem;
  font-weight: 600;
  white-space: nowrap;
  color: var(--text);
}
.icon.warning { color: var(--warn); }
.icon.critical { color: var(--bad); }
</style>
