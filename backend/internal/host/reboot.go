package host

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Reboot tells whether the running NixOS system differs from the one that
// was booted in a way only a reboot applies: the kernel, its modules or the
// initrd (the same check as nixos-rebuild's "reboot required" tools).
type Reboot struct {
	Required bool `json:"required"`
	// Changed lists what differs: "kernel", "initrd", "kernel-modules".
	Changed []string `json:"changed"`
	// BootedKernel and CurrentKernel are package names like "linux-6.18.60".
	BootedKernel  string `json:"bootedKernel"`
	CurrentKernel string `json:"currentKernel"`
}

var rebootLinks = []string{"kernel", "initrd", "kernel-modules"}

func (r *Reader) run(name string) string {
	root := r.RunRoot
	if root == "" {
		root = "/run"
	}
	return filepath.Join(root, name)
}

// reboot compares /run/booted-system with /run/current-system; nil when
// either is missing (not NixOS).
func (r *Reader) reboot() *Reboot {
	booted, current := r.run("booted-system"), r.run("current-system")
	if _, err := os.Stat(booted); err != nil {
		return nil
	}
	if _, err := os.Stat(current); err != nil {
		return nil
	}
	rb := &Reboot{Changed: []string{}}
	for _, link := range rebootLinks {
		b, berr := os.Readlink(filepath.Join(booted, link))
		c, cerr := os.Readlink(filepath.Join(current, link))
		if berr != nil && cerr != nil {
			continue // e.g. a container without its own kernel
		}
		if b != c {
			rb.Changed = append(rb.Changed, link)
		}
		if link == "kernel" {
			rb.BootedKernel, rb.CurrentKernel = storeName(b), storeName(c)
		}
	}
	rb.Required = len(rb.Changed) > 0
	return rb
}

var storeHash = regexp.MustCompile(`^[0-9a-z]{32}-`)

// storeName turns "/nix/store/<hash>-linux-6.18.60/bzImage" into
// "linux-6.18.60".
func storeName(path string) string {
	rest, ok := strings.CutPrefix(path, "/nix/store/")
	if !ok {
		return ""
	}
	dir, _, _ := strings.Cut(rest, "/")
	return storeHash.ReplaceAllString(dir, "")
}

func (r *Reader) uptime() float64 {
	b, err := os.ReadFile(r.proc("uptime"))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}
