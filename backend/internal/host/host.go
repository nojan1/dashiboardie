// Package host reports overall machine resource usage (CPU, memory, mounted
// filesystems) straight from /proc and statfs, without external tools.
package host

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Stats struct {
	Hostname    string       `json:"hostname"`
	CPU         CPU          `json:"cpu"`
	Memory      Memory       `json:"memory"`
	Filesystems []Filesystem `json:"filesystems"`
}

type CPU struct {
	// Percent is busy time across all cores since the previous request
	// (0-100); nil if it could not be sampled.
	Percent *float64  `json:"percent"`
	Cores   int       `json:"cores"`
	Load    []float64 `json:"load"` // 1, 5 and 15 minute load averages
}

type Memory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	UsedBytes      uint64 `json:"usedBytes"` // total minus MemAvailable
	SwapTotalBytes uint64 `json:"swapTotalBytes"`
	SwapUsedBytes  uint64 `json:"swapUsedBytes"`
}

type Filesystem struct {
	Mountpoint string `json:"mountpoint"`
	Source     string `json:"source"`
	FSType     string `json:"fsType"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
	AvailBytes uint64 `json:"availBytes"`
	// Error is set when statfs failed or timed out (e.g. a hung NFS mount).
	Error string `json:"error,omitempty"`
}

// realFSTypes are the filesystem types worth showing; everything else
// (proc, tmpfs, cgroup, overlay, FUSE helpers, ...) is skipped.
var realFSTypes = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true,
	"zfs": true, "bcachefs": true, "f2fs": true, "vfat": true, "exfat": true,
	"ntfs": true, "ntfs3": true, "fuseblk": true,
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true,
}

const statfsTimeout = 2 * time.Second

// Reader collects Stats. The zero value reads the live system.
type Reader struct {
	// ProcRoot defaults to /proc; tests point it at fixtures.
	ProcRoot string
	// Statfs defaults to syscall.Statfs.
	Statfs func(path string, st *syscall.Statfs_t) error

	mu      sync.Mutex
	prevCPU *cpuTimes
}

func (r *Reader) proc(name string) string {
	root := r.ProcRoot
	if root == "" {
		root = "/proc"
	}
	return filepath.Join(root, name)
}

func (r *Reader) Read(ctx context.Context) (Stats, error) {
	var s Stats
	var err error
	s.Hostname, _ = os.Hostname()
	if s.CPU, err = r.readCPU(ctx); err != nil {
		return s, err
	}
	if s.Memory, err = r.readMemory(); err != nil {
		return s, err
	}
	if s.Filesystems, err = r.readFilesystems(); err != nil {
		return s, err
	}
	return s, nil
}

// --- CPU ---

type cpuTimes struct{ busy, total uint64 }

func (r *Reader) readCPU(ctx context.Context) (CPU, error) {
	cur, cores, err := r.cpuTimes()
	if err != nil {
		return CPU{}, err
	}
	r.mu.Lock()
	prev := r.prevCPU
	r.prevCPU = &cur
	r.mu.Unlock()

	// On the first request there is nothing to diff against, so take a
	// second sample shortly after.
	if prev == nil {
		select {
		case <-time.After(250 * time.Millisecond):
		case <-ctx.Done():
			return CPU{}, ctx.Err()
		}
		prev = &cur
		if cur, _, err = r.cpuTimes(); err != nil {
			return CPU{}, err
		}
		r.mu.Lock()
		r.prevCPU = &cur
		r.mu.Unlock()
	}

	c := CPU{Cores: cores, Load: r.loadAvg()}
	if dt := cur.total - prev.total; cur.total > prev.total && cur.busy >= prev.busy {
		pct := float64(cur.busy-prev.busy) / float64(dt) * 100
		c.Percent = &pct
	}
	return c, nil
}

func (r *Reader) cpuTimes() (cpuTimes, int, error) {
	f, err := os.Open(r.proc("stat"))
	if err != nil {
		return cpuTimes{}, 0, err
	}
	defer f.Close()
	return parseStat(f)
}

// parseStat reads the aggregate "cpu" line of /proc/stat and counts the
// per-core "cpuN" lines.
func parseStat(rd io.Reader) (cpuTimes, int, error) {
	var t cpuTimes
	found := false
	cores := 0
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			cores++
			continue
		}
		// user nice system idle iowait irq softirq steal (guest time is
		// already included in user/nice).
		for i := 1; i < len(f) && i <= 8; i++ {
			v, err := strconv.ParseUint(f[i], 10, 64)
			if err != nil {
				return t, 0, fmt.Errorf("parse /proc/stat: %w", err)
			}
			t.total += v
			if i != 4 && i != 5 { // idle, iowait
				t.busy += v
			}
		}
		found = true
	}
	if !found {
		return t, 0, fmt.Errorf("parse /proc/stat: no cpu line")
	}
	return t, cores, sc.Err()
}

func (r *Reader) loadAvg() []float64 {
	b, err := os.ReadFile(r.proc("loadavg"))
	if err != nil {
		return nil
	}
	f := strings.Fields(string(b))
	var out []float64
	for i := 0; i < 3 && i < len(f); i++ {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// --- Memory ---

func (r *Reader) readMemory() (Memory, error) {
	f, err := os.Open(r.proc("meminfo"))
	if err != nil {
		return Memory{}, err
	}
	defer f.Close()
	return parseMeminfo(f)
}

func parseMeminfo(rd io.Reader) (Memory, error) {
	kb := map[string]uint64{}
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		if n, err := strconv.ParseUint(f[0], 10, 64); err == nil {
			kb[k] = n
		}
	}
	if err := sc.Err(); err != nil {
		return Memory{}, err
	}
	total, ok := kb["MemTotal"]
	if !ok {
		return Memory{}, fmt.Errorf("parse meminfo: no MemTotal")
	}
	avail := kb["MemAvailable"]
	m := Memory{
		TotalBytes:     total * 1024,
		UsedBytes:      (total - min(avail, total)) * 1024,
		SwapTotalBytes: kb["SwapTotal"] * 1024,
		SwapUsedBytes:  (kb["SwapTotal"] - min(kb["SwapFree"], kb["SwapTotal"])) * 1024,
	}
	return m, nil
}

// --- Filesystems ---

type mount struct {
	device, mountpoint, fsType, source string
}

func (r *Reader) readFilesystems() ([]Filesystem, error) {
	f, err := os.Open(r.proc("self/mountinfo"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	mounts, err := parseMountinfo(f)
	if err != nil {
		return nil, err
	}

	out := make([]Filesystem, len(mounts))
	var wg sync.WaitGroup
	for i, m := range mounts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = r.statfs(m)
		}()
	}
	wg.Wait()

	// Drop pseudo-empty filesystems (e.g. some FUSE mounts report 0 blocks).
	kept := out[:0]
	for _, fs := range out {
		if fs.TotalBytes > 0 || fs.Error != "" {
			kept = append(kept, fs)
		}
	}
	return kept, nil
}

// parseMountinfo returns one mount per device, keeping the shortest
// mountpoint, so bind mounts (/nix/store on NixOS) and btrfs subvolumes of the
// same filesystem appear once.
func parseMountinfo(rd io.Reader) ([]mount, error) {
	byDevice := map[string]mount{}
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		// id parent major:minor root mountpoint opts [optional...] - fstype source superopts
		f := strings.Fields(sc.Text())
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+2 >= len(f) {
			continue
		}
		m := mount{
			device:     f[2],
			mountpoint: unescape(f[4]),
			fsType:     f[sep+1],
			source:     unescape(f[sep+2]),
		}
		if !realFSTypes[m.fsType] {
			continue
		}
		if prev, ok := byDevice[m.device]; ok && len(prev.mountpoint) <= len(m.mountpoint) {
			continue
		}
		byDevice[m.device] = m
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	mounts := make([]mount, 0, len(byDevice))
	for _, m := range byDevice {
		mounts = append(mounts, m)
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].mountpoint < mounts[j].mountpoint })
	return mounts, nil
}

// unescape decodes the octal escapes mountinfo uses for spaces etc. ("\040").
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (r *Reader) statfs(m mount) Filesystem {
	fs := Filesystem{Mountpoint: m.mountpoint, Source: m.source, FSType: m.fsType}
	statfs := r.Statfs
	if statfs == nil {
		statfs = syscall.Statfs
	}
	type result struct {
		st  syscall.Statfs_t
		err error
	}
	// statfs on a dead network mount can block indefinitely; give up on it
	// rather than stall the whole response. The goroutine is left to finish.
	done := make(chan result, 1)
	go func() {
		var st syscall.Statfs_t
		err := statfs(m.mountpoint, &st)
		done <- result{st, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			fs.Error = res.err.Error()
			return fs
		}
		bs := uint64(res.st.Bsize)
		fs.TotalBytes = res.st.Blocks * bs
		fs.UsedBytes = (res.st.Blocks - res.st.Bfree) * bs
		fs.AvailBytes = res.st.Bavail * bs
	case <-time.After(statfsTimeout):
		fs.Error = "timed out"
	}
	return fs
}
