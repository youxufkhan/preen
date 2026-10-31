// Package safe is the single gate every Preen delete goes through.
// Design follows Mole's docs/SECURITY_DESIGN.md (Layer 1), translated to Linux.
package safe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

var home, _ = os.UserHomeDir()

// Checked before the allow list: live session sockets inside /tmp.
var denyFirst = []string{"/tmp/.X11-unix", "/tmp/.ICE-unix", "/tmp/.XIM-unix", "/tmp/.font-unix"}

// Strictly below these is deletable even though /var or a top-level rule would refuse it.
var allowUnder = []string{
	"/tmp", "/var/tmp", "/var/cache", "/var/crash",
	"/var/lib/systemd/coredump", "/var/lib/snapd/cache",
}

// The path and everything below it is refused.
var denyUnder = []string{
	"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/usr", "/etc", "/boot",
	"/proc", "/sys", "/dev", "/run", "/var", "/snap", "/root", "/srv",
}

// Only the bare path is refused; children stay deletable (empty-variable collapse guard).
var denyBare = []string{
	"/usr/local", "/usr/local/bin", "/usr/local/sbin", "/usr/local/lib", "/usr/local/share",
	"/usr/local/etc", "/usr/local/include", "/usr/local/src", "/usr/local/games", "/usr/local/man",
}

// Relative to $HOME. Bare dirs: refuse the dir itself. Secret dirs: refuse everything below.
var homeBare = []string{
	".config", ".local", ".local/share", ".local/state", ".local/bin", ".cache", "snap", ".var", ".var/app",
	"Desktop", "Documents", "Downloads", "Music", "Pictures", "Public", "Templates", "Videos",
}
var homeSecret = []string{
	".ssh", ".gnupg", ".pki", ".password-store", ".local/share/keyrings", ".local/share/kwalletd",
}

// Browser credential stores, refused wherever they appear.
var secretNames = map[string]bool{
	"Cookies": true, "Login Data": true, "key4.db": true, "logins.json": true, "cert9.db": true,
}

// ValidateDelete returns nil only when path may be deleted.
func ValidateDelete(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("refused %q: not an absolute path", path)
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return fmt.Errorf("refused %q: control character", path)
		}
	}
	for _, c := range strings.Split(path, "/") {
		if c == ".." {
			return fmt.Errorf("refused %q: '..' component", path)
		}
	}
	p := filepath.Clean(path)
	if err := check(p); err != nil {
		return fmt.Errorf("refused %q: %w", path, err)
	}
	// Symlinks can only add a refusal, never grant one. Check both where the
	// entry itself lives (resolved parent) and what it points to (resolved leaf).
	if dir, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		if err := check(filepath.Join(dir, filepath.Base(p))); err != nil {
			return fmt.Errorf("refused %q: parent resolves to protected path: %w", path, err)
		}
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		if err := check(r); err != nil {
			return fmt.Errorf("refused %q: resolves to protected path: %w", path, err)
		}
	}
	return nil
}

func check(p string) error {
	if filepath.Dir(p) == "/" || p == "/" {
		return fmt.Errorf("top-level path")
	}
	if home != "" && (p == home || under(home, p)) {
		return fmt.Errorf("home directory or its parent")
	}
	if filepath.Dir(p) == "/home" {
		return fmt.Errorf("a user's home directory")
	}
	if secretNames[filepath.Base(p)] {
		return fmt.Errorf("credential store")
	}
	for _, d := range denyFirst {
		if p == d || under(p, d) {
			return fmt.Errorf("live session socket dir")
		}
	}
	for _, a := range allowUnder {
		if under(p, a) {
			return nil
		}
	}
	for _, d := range denyBare {
		if p == d {
			return fmt.Errorf("protected root")
		}
	}
	for _, d := range denyUnder {
		if (p == d || under(p, d)) && !(d == "/usr" && under(p, "/usr/local")) {
			return fmt.Errorf("system path")
		}
	}
	if home != "" {
		for _, b := range homeBare {
			if p == filepath.Join(home, b) {
				return fmt.Errorf("protected home folder")
			}
		}
		for _, s := range homeSecret {
			if d := filepath.Join(home, s); p == d || under(p, d) {
				return fmt.Errorf("secrets folder")
			}
		}
	}
	return nil
}

// under reports whether p is strictly below dir.
func under(p, dir string) bool { return strings.HasPrefix(p, dir+"/") }

// Remove validates path, then deletes it permanently.
// ponytail: validate-then-remove has a TOCTOU window; exploiting it needs write access to your own dirs.
func Remove(path string) error {
	if err := ValidateDelete(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Trash validates path, then moves it to the freedesktop Trash.
// ponytail: shells out to gio (libglib2.0-bin, present on Ubuntu and Kubuntu); add kioclient fallback if a distro lacks it.
func Trash(path string) error {
	if err := ValidateDelete(path); err != nil {
		return err
	}
	if out, err := exec.Command("gio", "trash", "--", path).CombinedOutput(); err != nil {
		return fmt.Errorf("trash %q: %v: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}
