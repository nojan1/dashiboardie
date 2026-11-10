<script setup lang="ts">
import { computed } from 'vue'
import { usageLevel } from '../format'

// A single ratio against a limit. The track is a tint of the fill's own hue.
// Above the thresholds the fill switches to a status colour; the caller shows
// the matching <UsageFlag> as text so the state is never colour alone.
const props = defineProps<{ percent: number | null; label: string }>()

const level = computed(() => usageLevel(props.percent))
const width = computed(() => `${Math.min(100, Math.max(0, props.percent ?? 0))}%`)
</script>

<template>
  <div
    class="meter"
    :class="level"
    role="meter"
    :aria-label="label"
    aria-valuemin="0"
    aria-valuemax="100"
    :aria-valuenow="percent ?? undefined"
  >
    <div class="fill" :style="{ width }"></div>
  </div>
</template>

<style scoped>
.meter {
  --fill: var(--accent);
  height: 8px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--fill) 18%, transparent);
  overflow: hidden;
}
.meter.warning { --fill: var(--warn); }
.meter.critical { --fill: var(--bad); }
.fill {
  height: 100%;
  border-radius: 4px;
  background: var(--fill);
  transition: width 0.4s ease;
}
</style>
