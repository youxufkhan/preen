package safe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Must be refused. Linux translation of Mole's tests/fuzz_corpus/dangerous_paths.txt.
func dangerous() []string {
	return []string{
		"", "relative/path", "./foo", "~/Documents", "just-a-name", ".Trash",
		"/", "/bin", "/sbin", "/usr", "/usr/bin", "/usr/lib", "/usr/share", "/etc", "/var", "/boot",
		"/dev", "/dev/null", "/proc", "/proc/self/root", "/sys", "/run", "/root", "/snap", "/srv",
		"/bin/bash", "/sbin/init", "/usr/bin/ls", "/etc/passwd", "/etc/sudoers", "/boot/vmlinuz",
		"/var/lib/dpkg", "/var/lib/dpkg/status", "/var/log", "/var/lib/snapd/snaps",
		"/../etc/passwd", "/home/../etc/passwd", "/home/me/../../etc", "/var/../etc/passwd",
		"/tmp/../etc", "/var/cache/..", "/etc/..",
		"/home/me/foo\tbar", "/home/me/foo\nbar",
		"//etc/passwd", "///usr", "/etc/./passwd", "/etc/.",
		"/home", "/home/me", "/opt", "/media", "/mnt", "/tmp", "/swap.img", "/lost+found",
		"/usr/local", "/usr/local/bin", "/usr/local/share",
		"/var/cache", "/var/tmp", "/var/lib/systemd/coredump",
		"/tmp/.X11-unix", "/tmp/.X11-unix/X0", "/tmp/.ICE-unix",
		home, filepath.Dir(home), home + "/", home + "/.config", home + "/.cache", home + "/.local/share",
		home + "/Documents", home + "/Downloads", home + "/snap",
		home + "/.ssh", home + "/.ssh/id_ed25519", home + "/.gnupg/private-keys-v1.d",
		home + "/.local/share/keyrings/login.keyring",
		home + "/.config/google-chrome/Default/Cookies", home + "/.config/google-chrome/Default/Login Data",
		home + "/.mozilla/firefox/x.default/key4.db", home + "/.mozilla/firefox/x.default/logins.json",
	}
}

// Must be allowed: the places Preen actually cleans.
func cleanable() []string {
	return []string{
		home + "/.cache/google-chrome/Default/Cache", home + "/.cache/pip", home + "/.cache/thumbnails/large",
		home + "/.local/share/Trash/files/old.iso", home + "/.gradle/caches", home + "/.npm/_cacache",
		home + "/Downloads/setup.deb", home + "/.config/someapp", home + "/snap/thunderbird",
		"/var/cache/apt/archives/foo.deb", "/var/crash/_usr_bin_x.1000.crash",
		"/var/lib/systemd/coredump/core.x.zst", "/var/lib/snapd/cache/abc", "/var/tmp/old",
		"/tmp/preen-test", "/opt/someapp", "/usr/local/bin/sometool", "/usr/local/someapp",
	}
}

func TestDangerousRefused(t *testing.T) {
	for _, p := range dangerous() {
		if ValidateDelete(p) == nil {
			t.Errorf("allowed dangerous path %q", p)
		}
	}
}

func TestCleanableAllowed(t *testing.T) {
	for _, p := range cleanable() {
		if err := ValidateDelete(p); err != nil {
			t.Errorf("refused cleanable path: %v", err)
		}
	}
}

func TestSymlinksCannotEscape(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "etc-link")
	if err := os.Symlink("/etc", link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{link, link + "/passwd", link + "/does-not-exist"} {
		if ValidateDelete(p) == nil {
			t.Errorf("allowed %q through a symlink to /etc", p)
		}
	}
	// A plain file next to the link stays deletable.
	ok := filepath.Join(dir, "plain")
	os.WriteFile(ok, nil, 0o600)
	if err := ValidateDelete(ok); err != nil {
		t.Errorf("refused plain temp file: %v", err)
	}
}

func TestRemoveDeletesOnlyValidated(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x")
	os.WriteFile(f, []byte("x"), 0o600)
	if err := Remove(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file still exists after Remove")
	}
	if Remove("/etc/passwd") == nil {
		t.Fatal("Remove accepted /etc/passwd")
	}
}

// Property: whatever the input, an accepted path is never a system root, a
// top-level entry, an ancestor of $HOME, or inside a secrets folder.
func FuzzValidateDelete(f *testing.F) {
	for _, p := range append(dangerous(), cleanable()...) {
		f.Add(p)
	}
	critical := []string{"/bin", "/sbin", "/lib", "/lib64", "/etc", "/boot", "/proc", "/sys", "/dev", "/run", "/root", "/snap", "/srv"}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateDelete(s) != nil {
			return
		}
		if !filepath.IsAbs(s) || strings.Contains("/"+s+"/", "/../") {
			t.Fatalf("accepted non-absolute or '..' path %q", s)
		}
		p := filepath.Clean(s)
		if p == "/" || filepath.Dir(p) == "/" || p == home || strings.HasPrefix(home+"/", p+"/") {
			t.Fatalf("accepted root, top-level or home ancestor %q", s)
		}
		for _, c := range critical {
			if p == c || strings.HasPrefix(p, c+"/") {
				t.Fatalf("accepted system path %q", s)
			}
		}
		if strings.HasPrefix(p, "/usr/") && !strings.HasPrefix(p, "/usr/local/") {
			t.Fatalf("accepted /usr path %q", s)
		}
		if p == home+"/.ssh" || p == home+"/.gnupg" || strings.HasPrefix(p, home+"/.ssh/") || strings.HasPrefix(p, home+"/.gnupg/") {
			t.Fatalf("accepted secrets path %q", s)
		}
	})
}
