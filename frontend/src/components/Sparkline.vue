<script setup lang="ts">
import { computed, ref } from 'vue'
import { formatDuration } from '../format'

// A single-series 0-100% line over a time window ending at the latest sample.
// The x axis is time, so gaps (a missing sample) break the line instead of
// bridging it. While history is still filling up (after a restart) the
// window covers what there is, at least MIN_SPAN, instead of squeezing a
// few minutes against the right edge of an hour.
// Hover or arrow keys move a crosshair that snaps to the nearest sample.
const props = defineProps<{
  points: { t: number; v: number | null }[]
  windowSeconds: number
  label: string // e.g. "CPU", used in the accessible summary and tooltip
}>()

const W = 1000
const H = 100
const MIN_SPAN = 5 * 60

const end = computed(() => props.points.at(-1)?.t ?? Date.now() / 1000)
const spanSeconds = computed(() => {
  const covered = end.value - (props.points[0]?.t ?? end.value)
  return Math.min(props.windowSeconds, Math.max(MIN_SPAN, covered))
})
const start = computed(() => end.value - spanSeconds.value)
const x = (t: number) => ((t - start.value) / spanSeconds.value) * W
// Leave a little headroom so a line at 100% isn't clipped by the stroke.
const y = (v: number) => H - (Math.min(100, Math.max(0, v)) / 100) * (H - 4) - 2

const visible = computed(() => props.points.filter((p) => p.t >= start.value))

// One path segment per run of non-null samples.
const segments = computed(() => {
  const segs: { t: number; v: number }[][] = []
  let cur: { t: number; v: number }[] = []
  for (const p of visible.value) {
    if (p.v == null) {
      if (cur.length) segs.push(cur)
      cur = []
    } else cur.push({ t: p.t, v: p.v })
  }
  if (cur.length) segs.push(cur)
  return segs
})

const linePath = computed(() =>
  segments.value
    .map((s) => s.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(''))
    .join(''),
)
const areaPath = computed(() =>
  segments.value
    .filter((s) => s.length > 1)
    .map((s) => {
      const line = s.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join('')
      return `${line}L${x(s.at(-1)!.t).toFixed(1)},${H}L${x(s[0].t).toFixed(1)},${H}Z`
    })
    .join(''),
)

const values = computed(() => visible.value.map((p) => p.v).filter((v): v is number => v != null))
const peak = computed(() => (values.value.length ? Math.max(...values.value) : null))
const avg = computed(() =>
  values.value.length ? values.value.reduce((a, b) => a + b, 0) / values.value.length : null,
)
const span = computed(() => formatDuration(spanSeconds.value))
const summary = computed(() =>
  peak.value == null
    ? `${props.label} over the last ${span.value}: no data yet`
    : `${props.label} over the last ${span.value}: average ${Math.round(avg.value!)}%, peak ${Math.round(peak.value)}%`,
)

// --- hover / keyboard crosshair ---
const active = ref<number | null>(null) // index into visible

function nearest(t: number) {
  let best = -1
  let bestDist = Infinity
  visible.value.forEach((p, i) => {
    const d = Math.abs(p.t - t)
    if (d < bestDist) {
      best = i
      bestDist = d
    }
  })
  return best < 0 ? null : best
}

function onMove(e: PointerEvent) {
  const box = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const frac = (e.clientX - box.left) / box.width
  active.value = nearest(start.value + frac * spanSeconds.value)
}

function onKey(e: KeyboardEvent) {
  const n = visible.value.length
  if (!n) return
  if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
    e.preventDefault()
    const step = e.key === 'ArrowLeft' ? -1 : 1
    active.value = Math.min(n - 1, Math.max(0, (active.value ?? n) + step))
  } else if (e.key === 'Escape') {
    active.value = null
  }
}

const hovered = computed(() => (active.value == null ? null : visible.value[active.value] ?? null))
const hoverLeft = computed(() => (hovered.value ? (x(hovered.value.t) / W) * 100 : 0))
const hoverTop = computed(() => (hovered.value?.v != null ? (y(hovered.value.v) / H) * 100 : null))
const hoverTime = computed(() =>
  hovered.value ? new Date(hovered.value.t * 1000).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }) : '',
)
</script>

<template>
  <div class="spark">
    <div
      class="plot"
      role="img"
      tabindex="0"
      :aria-label="summary"
      @pointermove="onMove"
      @pointerleave="active = null"
      @blur="active = null"
      @keydown="onKey"
    >
      <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" aria-hidden="true">
        <line class="base" x1="0" :y1="H" :x2="W" :y2="H" />
        <path class="area" :d="areaPath" />
        <path class="line" :d="linePath" />
      </svg>
      <template v-if="hovered">
        <div class="crosshair" :style="{ left: `${hoverLeft}%` }"></div>
        <div v-if="hoverTop != null" class="dot" :style="{ left: `${hoverLeft}%`, top: `${hoverTop}%` }"></div>
        <div class="tip" :class="{ flip: hoverLeft > 60 }" :style="{ left: `${hoverLeft}%` }">
          <strong>{{ hovered.v == null ? '—' : `${Math.round(hovered.v)}%` }}</strong>
          <span>{{ label }} · {{ hoverTime }}</span>
        </div>
      </template>
    </div>
    <div class="axis" aria-hidden="true">
      <span>{{ span }} ago</span>
      <span v-if="peak != null">peak {{ Math.round(peak) }}%</span>
      <span>now</span>
    </div>
  </div>
</template>

<style scoped>
.spark { margin-top: 0.15rem; }
.plot {
  position: relative;
  height: 44px;
  cursor: crosshair;
  touch-action: pan-y;
  border-radius: 2px;
}
.plot:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
svg {
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
}
.base { stroke: var(--border); stroke-width: 1; vector-effect: non-scaling-stroke; }
.line {
  fill: none;
  stroke: var(--accent);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
  vector-effect: non-scaling-stroke;
}
.area { fill: color-mix(in srgb, var(--accent) 14%, transparent); stroke: none; }
.crosshair {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: var(--muted);
  opacity: 0.6;
  pointer-events: none;
}
.dot {
  position: absolute;
  width: 8px;
  height: 8px;
  margin: -4px 0 0 -4px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 2px var(--surface);
  pointer-events: none;
}
.tip {
  position: absolute;
  bottom: calc(100% + 6px);
  transform: translateX(8px);
  display: flex;
  gap: 0.4rem;
  align-items: baseline;
  padding: 0.2rem 0.45rem;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--surface);
  box-shadow: 0 2px 8px rgb(0 0 0 / 0.15);
  font-size: 0.75rem;
  white-space: nowrap;
  pointer-events: none;
  z-index: 1;
}
.tip.flip { transform: translateX(calc(-100% - 8px)); }
.tip strong { color: var(--text); font-variant-numeric: tabular-nums; }
.tip span { color: var(--muted); }
.axis {
  display: flex;
  justify-content: space-between;
  margin-top: 0.2rem;
  font-size: 0.7rem;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
}
</style>
