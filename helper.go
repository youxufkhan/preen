package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"preen/internal/safe"
)

// runHelper runs `preen helper <actions...>` as root through pkexec (one
// password prompt for the whole batch) and returns each action's result.
func runHelper(actions []string) (map[string]Event, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := append([]string{"helper"}, actions...)
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(self, args...)
	} else {
		cmd = exec.Command("pkexec", append([]string{self}, args...)...)
	}
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	results := map[string]Event{}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Type == "result" {
			results[e.ID] = e
		}
	}
	err = cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) && len(results) == 0 {
		return results, errors.New("administrator authentication was cancelled")
	}
	return results, err
}

// helperActions is the complete list of things Preen ever does as root.
// Each takes an optional argument and re-checks system state itself.
var helperActions = map[string]func(arg string) error{
	"apt-clean":      func(string) error { return run("apt-get", "clean") },
	"journal-vacuum": func(string) error { return run("journalctl", fmt.Sprintf("--vacuum-size=%dM", journalKeep>>20)) },
	"snap-disabled": func(string) error {
		var errs []error
		for _, s := range snapDisabled() {
			errs = append(errs, run("snap", "remove", s[0], "--revision="+s[1]))
		}
		return errors.Join(errs...)
	},
	"crash-dumps": func(string) error {
		var errs []error
		for _, p := range append(children("/var/crash"), children("/var/lib/systemd/coredump")...) {
			errs = append(errs, safe.Remove(p))
		}
		return errors.Join(errs...)
	},
	"old-kernels": func(string) error {
		_, pkgs := oldKernels()
		if len(pkgs) == 0 {
			return nil
		}
		return run("apt-get", append([]string{"-y", "purge"}, pkgs...)...)
	},
	"autoremove": func(string) error { return run("apt-get", "-y", "autoremove", "--purge") },
	"rc-configs": func(string) error {
		pk := rcPackages()
		if len(pk) == 0 {
			return nil
		}
		return run("dpkg", append([]string{"--purge"}, pk...)...)
	},
	"dns-flush":   func(string) error { return run("resolvectl", "flush-caches") },
	"fstrim":      func(string) error { return run("fstrim", "-av") },
	"snap-retain": func(string) error { return run("snap", "set", "system", "refresh.retain=2") },
	"pkg-remove": func(pkg string) error {
		if err := checkRemovablePackage(pkg); err != nil {
			return err
		}
		return run("apt-get", "-y", "purge", pkg)
	},
	"snap-remove": func(name string) error {
		if err := checkRemovableSnap(name); err != nil {
			return err
		}
		return run("snap", "remove", "--purge", name)
	},
	"flatpak-remove": func(id string) error {
		if !slices.Contains(strings.Split(cmdOut("flatpak", "list", "--system", "--app", "--columns=application"), "\n"), id) {
			return fmt.Errorf("%q is not an installed system flatpak", id)
		}
		return run("flatpak", "uninstall", "--system", "-y", "--delete-data", id)
	},
}

func helperMain(args []string) int {
	jsonOut = true
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "preen helper must run as root (it is started through pkexec)")
		return 1
	}
	os.Setenv("DEBIAN_FRONTEND", "noninteractive")
	for _, a := range args {
		name, arg, _ := strings.Cut(a, ":")
		fn, ok := helperActions[name]
		if !ok {
			emit(Event{Type: "result", ID: a, Status: "failed", Message: "unknown action"})
			continue
		}
		if err := fn(arg); err != nil {
			emit(Event{Type: "result", ID: a, Status: "failed", Message: err.Error()})
		} else {
			emit(Event{Type: "result", ID: a, Status: "applied"})
		}
	}
	return 0
}

// run executes a command, returning its last output line as the error on failure.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("%s: %s", name, lines[len(lines)-1])
	}
	return nil
}

// Packages whose removal breaks the desktop or the system.
var protectedPkgs = []string{
	"*ubuntu-desktop*", "kde-plasma-desktop", "kde-standard", "kde-full", "task-*", "ubuntu-*", "kubuntu-*", "plasma-*", "kwin*", "sddm*", "gdm3", "gnome-shell*", "systemsettings",
	"systemd*", "linux-*", "snapd", "apt*", "dpkg", "sudo", "polkit*", "pkexec", "network-manager*", "pipewire*",
	"wireplumber", "xorg", "xwayland", "dbus*", "libc6", "grub*", "shim*", "base-files", "coreutils", "bash",
}

// Debian package names: lowercase letters, digits, '+', '-', '.'. It must end in a
// letter or digit, because apt reads a trailing '+' as "install" and '-' as "remove",
// and it must hold no glob or pattern characters, because apt and dpkg-query expand those.
var debName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*[a-z0-9]$`)

func checkRemovablePackage(pkg string) error {
	if !debName.MatchString(pkg) {
		return fmt.Errorf("invalid package name %q", pkg)
	}
	if cmdOut("dpkg-query", "-W", "-f=${Package}\n", pkg) != pkg {
		return fmt.Errorf("%s is not installed", pkg)
	}
	removed := simulateRemoval(pkg)
	if len(removed) == 0 {
		return fmt.Errorf("%s is not installed", pkg)
	}
	// Every package apt would take with it must pass the same checks as the named one.
	info := cmdOutAny("dpkg-query", append([]string{"-W", "-f=${Package}|${db:Status-Abbrev}|${Priority}|${Essential}\n"}, removed...)...)
	for _, l := range strings.Split(info, "\n") {
		f := strings.Split(l, "|")
		if len(f) != 4 {
			continue
		}
		name := strings.SplitN(f[0], ":", 2)[0]
		if f[3] == "yes" || f[2] == "required" || f[2] == "important" || matchesAny(name, protectedPkgs) {
			if name == pkg {
				return fmt.Errorf("%s is part of the system and is not removed", pkg)
			}
			return fmt.Errorf("removing %s would also remove %s", pkg, name)
		}
	}
	return nil
}

// simulateRemoval lists every package `apt-get purge pkg` would remove.
func simulateRemoval(pkg string) []string {
	var out []string
	for _, l := range strings.Split(cmdOut("apt-get", "-s", "purge", pkg), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && (f[0] == "Remv" || f[0] == "Purg") {
			out = append(out, f[1])
		}
	}
	return out
}

var snapName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

var protectedSnaps = []string{"core*", "snapd", "bare", "gnome-*", "gtk-common-themes", "kf5-*", "kf6-*", "mesa-*", "firmware-updater", "snap-store"}

func checkRemovableSnap(name string) error {
	if !snapName.MatchString(name) || matchesAny(name, protectedSnaps) {
		return fmt.Errorf("snap %q is not removable by Preen", name)
	}
	for _, l := range strings.Split(cmdOut("snap", "list"), "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == name {
			return nil
		}
	}
	return fmt.Errorf("snap %q is not installed", name)
}
