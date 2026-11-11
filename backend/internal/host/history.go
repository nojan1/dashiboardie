package host

import (
	"context"
	"log/slog"
	"time"
)

// Point is one history sample. CPU is nil when it couldn't be computed.
type Point struct {
	Time          int64    `json:"t"` // unix seconds
	CPUPercent    *float64 `json:"cpu"`
	MemoryPercent float64  `json:"mem"`
}

// History is the recent CPU and memory usage, oldest point first.
type History struct {
	IntervalSeconds float64 `json:"intervalSeconds"`
	// WindowSeconds is how far back history goes once it has filled up.
	WindowSeconds float64 `json:"windowSeconds"`
	Points        []Point `json:"points"`
}

// Start samples CPU and memory every interval until ctx ends, keeping the
// last `keep` points in memory (nothing is persisted, so history starts
// over when the service restarts). While it runs, Read reports the latest
// sampled CPU % so the overview and the graph agree.
func (r *Reader) Start(ctx context.Context, interval time.Duration, keep int) {
	r.mu.Lock()
	r.sampling = true
	r.interval = interval
	r.keep = keep
	r.mu.Unlock()

	go func() {
		// Prime the CPU counters, then take the first point shortly after so
		// the overview has a CPU figure right away.
		r.sampleCPU()
		wait := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = interval
			r.addPoint(time.Now())
		}
	}()
}

func (r *Reader) addPoint(now time.Time) {
	p := Point{Time: now.Unix(), CPUPercent: r.sampleCPU()}
	if m, err := r.readMemory(); err != nil {
		slog.Warn("sample memory", "err", err)
	} else if m.TotalBytes > 0 {
		p.MemoryPercent = float64(m.UsedBytes) / float64(m.TotalBytes) * 100
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latestCPU = p.CPUPercent
	r.history = append(r.history, p)
	if over := len(r.history) - r.keep; over > 0 {
		// Copy instead of reslicing so the backing array doesn't grow forever.
		r.history = append([]Point(nil), r.history[over:]...)
	}
}

// sampleCPU returns busy % since the previous sample (nil for the first).
func (r *Reader) sampleCPU() *float64 {
	cur, _, err := r.cpuTimes()
	if err != nil {
		slog.Warn("sample cpu", "err", err)
		return nil
	}
	r.mu.Lock()
	prev := r.prevCPU
	r.prevCPU = &cur
	r.mu.Unlock()
	if prev == nil || cur.total <= prev.total || cur.busy < prev.busy {
		return nil
	}
	pct := float64(cur.busy-prev.busy) / float64(cur.total-prev.total) * 100
	return &pct
}

func (r *Reader) historySnapshot() *History {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sampling {
		return nil
	}
	return &History{
		IntervalSeconds: r.interval.Seconds(),
		WindowSeconds:   r.interval.Seconds() * float64(r.keep),
		Points:          append([]Point{}, r.history...),
	}
}
