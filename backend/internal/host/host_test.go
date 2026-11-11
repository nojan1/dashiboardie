package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Captured from a NixOS host: /nix/store is a bind mount of the root
// filesystem, gvfs is a FUSE helper, and proc/tmpfs are pseudo filesystems.
const mountinfo = `22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw
25 22 8:1 /nix/store /nix/store ro,relatime shared:2 - ext4 /dev/sda1 rw
30 22 259:5 / /home rw,relatime shared:3 - ext4 /dev/nvme1n1p3 rw
31 22 259:1 / /mnt/my\040disk rw,relatime shared:4 - ntfs3 /dev/nvme0n1p1 rw
40 22 0:51 / /run/user/1000/gvfs rw,nosuid shared:5 - fuse.gvfsd-fuse gvfsd-fuse rw
41 22 0:22 / /proc rw,nosuid shared:6 - proc proc rw
42 22 0:23 / /run rw,nosuid shared:7 - tmpfs tmpfs rw
`

func TestParseMountinfo(t *testing.T) {
	mounts, err := parseMountinfo(strings.NewReader(mountinfo))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range mounts {
		got = append(got, m.mountpoint+"="+m.fsType)
	}
	want := "/=ext4,/home=ext4,/mnt/my disk=ntfs3"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

func TestParseStat(t *testing.T) {
	stat := "cpu  100 0 50 800 50 0 0 0 0 0\ncpu0 50 0 25 400 25 0 0 0 0 0\ncpu1 50 0 25 400 25 0 0 0 0 0\nintr 1\n"
	times, cores, err := parseStat(strings.NewReader(stat))
	if err != nil {
		t.Fatal(err)
	}
	if cores != 2 || times.total != 1000 || times.busy != 150 {
		t.Fatalf("got cores=%d %+v", cores, times)
	}
}

func TestParseMeminfo(t *testing.T) {
	m, err := parseMeminfo(strings.NewReader("MemTotal:  1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.TotalBytes != 1000*1024 || m.UsedBytes != 600*1024 || m.SwapUsedBytes != 50*1024 {
		t.Fatalf("got %+v", m)
	}
}

func TestReadCPUDiffsSuccessiveSamples(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu  100 0 0 900 0 0 0 0\ncpu0 100 0 0 900 0 0 0 0\n")
	write("loadavg", "0.50 0.25 0.10 1/100 123\n")
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 500 kB\n")
	write("self/mountinfo", "22 1 8:1 / / rw - ext4 /dev/sda1 rw\n")

	r := &Reader{
		ProcRoot: dir,
		Statfs: func(path string, st *syscall.Statfs_t) error {
			st.Bsize, st.Blocks, st.Bfree, st.Bavail = 4096, 100, 40, 30
			return nil
		},
	}
	ctx := context.Background()
	if _, err := r.Read(ctx); err != nil { // primes the CPU sample
		t.Fatal(err)
	}
	// 300 more busy ticks out of 400 total ticks = 75%.
	write("stat", "cpu  400 0 0 1000 0 0 0 0\ncpu0 400 0 0 1000 0 0 0 0\n")
	s, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.CPU.Percent == nil || *s.CPU.Percent != 75 {
		t.Fatalf("cpu percent: %v", s.CPU.Percent)
	}
	if len(s.CPU.Load) != 3 || s.CPU.Load[0] != 0.5 {
		t.Fatalf("load: %v", s.CPU.Load)
	}
	fs := s.Filesystems
	if len(fs) != 1 || fs[0].TotalBytes != 100*4096 || fs[0].UsedBytes != 60*4096 || fs[0].AvailBytes != 30*4096 {
		t.Fatalf("filesystems: %+v", fs)
	}
}

func TestReboot(t *testing.T) {
	run := t.TempDir()
	link := func(sys, name, target string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(run, sys), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(run, sys, name)); err != nil {
			t.Fatal(err)
		}
	}
	const old = "/nix/store/jyb28mnqm5vaxfggp513xkivabwqvkpy-linux-6.18.60/bzImage"
	const updated = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-linux-6.18.61/bzImage"
	link("booted-system", "kernel", old)
	link("booted-system", "initrd", "/nix/store/x-initrd/initrd")
	link("current-system", "kernel", old)
	link("current-system", "initrd", "/nix/store/x-initrd/initrd")

	r := &Reader{RunRoot: run}
	if rb := r.reboot(); rb == nil || rb.Required || rb.BootedKernel != "linux-6.18.60" {
		t.Fatalf("same system: %+v", rb)
	}
	os.Remove(filepath.Join(run, "current-system", "kernel"))
	link("current-system", "kernel", updated)
	rb := r.reboot()
	if rb == nil || !rb.Required || strings.Join(rb.Changed, ",") != "kernel" || rb.CurrentKernel != "linux-6.18.61" {
		t.Fatalf("new kernel: %+v", rb)
	}

	if rb := (&Reader{RunRoot: t.TempDir()}).reboot(); rb != nil {
		t.Fatalf("not NixOS: %+v", rb)
	}
}

func TestHistory(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu  100 0 0 900 0 0 0 0\n")
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 750 kB\n")
	r := &Reader{ProcRoot: dir, sampling: true, keep: 2, interval: 10 * time.Second}
	r.sampleCPU()
	for i, busy := range []int{200, 300, 400} {
		write("stat", fmt.Sprintf("cpu  %d 0 0 %d 0 0 0 0\n", busy, 900+100*(i+1)))
		r.addPoint(time.Unix(int64(i), 0))
	}
	h := r.historySnapshot()
	if len(h.Points) != 2 || h.Points[0].Time != 1 || h.IntervalSeconds != 10 {
		t.Fatalf("history: %+v", h)
	}
	p := h.Points[1]
	if p.CPUPercent == nil || *p.CPUPercent != 50 || p.MemoryPercent != 25 {
		t.Fatalf("point: cpu=%v mem=%v", p.CPUPercent, p.MemoryPercent)
	}
	if r.latestCPU == nil || *r.latestCPU != 50 {
		t.Fatalf("latest cpu: %v", r.latestCPU)
	}
}
