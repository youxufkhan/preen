package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// App is one installed application, found through its .desktop launcher.
type App struct {
	ID, Name, Icon, Source string // ID: apt:<pkg> | snap:<name> | flatpak:<id> | flatpak-user:<id> | appimage:<file> | local:<desktop>
	Bytes                  int64
	Installed, LastUsed    int64 // unix seconds; 0 = unknown
	Desktops               []string
	cmdline                string
	names                  map[string]bool // lowercase names used to find leftovers
	browser                bool
	protected              string
}

var sourceLabel = map[string]string{
	"apt": "Ubuntu package", "snap": "Snap", "flatpak": "Flatpak", "flatpak-user": "Flatpak (user)",
	"appimage": "AppImage", "local": "Launcher only",
}

func cmdUninstall(args []string) error {
	fs, o := planFlags("uninstall")
	list := fs.Bool("list", false, "list installed apps")
	sortBy := fs.String("sort", "name", "list order: name, size, installed (newest first) or used (least used first)")
	withData := fs.Bool("with-data", false, "also select settings and personal data (never browser profiles)")
	fs.Parse(args)
	progress("Finding installed apps")
	apps := listApps()
	clearProgress()
	if *list || fs.NArg() == 0 {
		sortApps(apps, *sortBy)
		for _, a := range apps {
			it := &Item{ID: a.ID, Group: sourceLabel[a.Source], Label: a.Name, Bytes: a.Bytes, Icon: a.Icon, Risk: "review",
				Note: a.protected, Disabled: a.protected != "", Installed: a.Installed, LastUsed: a.LastUsed}
			emit(Event{Type: "item", Item: it})
			note := ""
			if a.protected != "" {
				note = color("2", " · "+a.protected)
			}
			say("  %-34s %-15s %9s  %-14s %s%s", a.Name, sourceLabel[a.Source], human(a.Bytes), "used "+ago(a.LastUsed), color("2", a.ID), note)
		}
		emit(Event{Type: "done", Count: len(apps)})
		if !*list && !jsonOut {
			say("\nRun: preen uninstall <id>  (add --dry-run to preview)")
		}
		return nil
	}
	var app *App
	for _, a := range apps {
		if a.ID == fs.Arg(0) {
			app = a
		}
	}
	if app == nil {
		return fmt.Errorf("no installed app with id %q (see preen uninstall --list)", fs.Arg(0))
	}
	items := []*Item{removalItem(app)}
	items = append(items, leftovers(app, apps, *withData)...)
	return runPlan("uninstall", "Uninstall "+app.Name, items, o, true)
}

// removalItem is the step that removes the app itself; everything else depends on it.
func removalItem(a *App) *Item {
	it := &Item{ID: "app", Group: "Apps", Label: "Remove " + a.Name + " (" + sourceLabel[a.Source] + ")", Risk: "safe", Selected: true, required: true, Icon: a.Icon}
	kind, ref, _ := strings.Cut(a.ID, ":")
	switch kind {
	case "apt":
		it.Root, it.action = true, "pkg-remove:"+ref
		if err := checkRemovablePackage(ref); err != nil {
			it.Disabled, it.Selected, it.Note = true, false, err.Error()
		}
		it.Entries = pkgEntries(simulateRemoval(ref))
		if n := len(it.Entries) - 1; n > 0 && !it.Disabled {
			it.Note = fmt.Sprintf("apt also removes %s", plural(n, "dependent package"))
		}
	case "snap":
		it.Root, it.action = true, "snap-remove:"+ref
		if err := checkRemovableSnap(ref); err != nil {
			it.Disabled, it.Selected, it.Note = true, false, err.Error()
		}
		it.Entries = []Entry{{Path: "snap " + ref, Bytes: a.Bytes}}
	case "flatpak":
		it.Root, it.action = true, "flatpak-remove:"+ref
		it.Entries = []Entry{{Path: "flatpak " + ref, Bytes: a.Bytes}}
	case "flatpak-user":
		it.Entries = []Entry{{Path: "flatpak " + ref, Bytes: a.Bytes}}
		it.apply = func() error { return run("flatpak", "uninstall", "--user", "-y", "--delete-data", ref) }
	case "appimage":
		it.Entries = []Entry{{Path: ref, Bytes: a.Bytes}}
		for _, d := range a.Desktops {
			it.Entries = append(it.Entries, Entry{Path: d})
		}
	case "local":
		it.Label = "Remove " + a.Name + " launcher"
		it.Note = "installed manually: Preen removes the launcher, not the program files"
		for _, d := range a.Desktops {
			it.Entries = append(it.Entries, Entry{Path: d})
		}
	}
	it.Bytes = a.Bytes
	if kind == "apt" && len(it.Entries) > 0 {
		it.Bytes = sum(it.Entries)
	}
	return it
}

// Names too generic to match a leftover folder on their own.
var genericNames = map[string]bool{
	"app": true, "apps": true, "bin": true, "lib": true, "share": true, "common": true, "data": true, "config": true,
	"cache": true, "default": true, "applications": true, "autostart": true, "systemd": true, "java": true, "python": true,
	"python3": true, "node": true, "electron": true, "google": true, "microsoft": true, "kde": true, "gnome": true,
	"mozilla": true, "fonts": true, "icons": true, "themes": true, "env": true, "flatpak": true, "snap": true, "sh": true,
	"bash": true, "wine": true,
}

// leftovers finds files the app left in the user's home. Caches are selected;
// settings and data are kept unless withData (browser profiles always kept).
func leftovers(a *App, all []*App, withData bool) []*Item {
	if a.Source == "snap" || strings.HasPrefix(a.Source, "flatpak") {
		return nil // snapd and flatpak --delete-data remove their own data folders
	}
	others := map[string]string{}
	for _, o := range all {
		if o.ID != a.ID {
			for n := range o.names {
				others[n] = o.Name
			}
		}
	}
	var items []*Item
	addPath := func(p, kind string) {
		name := strings.ToLower(filepath.Base(p))
		it := &Item{ID: p, Group: "Leftovers", Label: tildePath(p), Risk: "safe", Selected: true}
		if kind == "data" {
			it.Risk, it.Selected = "personal", withData && !a.browser
			it.Label += "  (settings/data)"
		}
		for _, n := range []string{name, strings.TrimSuffix(name, "rc")} {
			if other, ok := others[n]; ok {
				it.Selected, it.Note = false, "also used by "+other
			}
		}
		it.Entries = entries([]string{p})
		if len(it.Entries) == 0 {
			it.Entries = []Entry{{Path: p}}
		}
		it.Bytes = sum(it.Entries)
		items = append(items, it)
	}
	for _, loc := range []struct{ dir, kind string }{
		{"~/.cache", "cache"}, {"~/.config", "data"}, {"~/.local/share", "data"}, {"~/.local/state", "data"},
	} {
		for _, p := range children(loc.dir) {
			n := strings.ToLower(filepath.Base(p))
			if a.names[n] || (loc.dir == "~/.config" && strings.HasSuffix(n, "rc") && a.names[strings.TrimSuffix(n, "rc")]) {
				addPath(p, loc.kind)
			}
		}
	}
	for _, p := range globs("~/.config/autostart/*.desktop") {
		if d, ok := parseDesktop(p); ok && (a.names[strings.ToLower(execBase(d.exec))] || a.names[strings.ToLower(d.id)]) {
			addPath(p, "launcher")
		}
	}
	if a.Source == "apt" {
		for _, d := range a.Desktops {
			if p := expand("~/.local/share/applications/" + filepath.Base(d)); exists(p) {
				addPath(p, "launcher")
			}
		}
	}
	return items
}

func tildePath(p string) string {
	if h := home(); strings.HasPrefix(p, h+"/") {
		return "~" + p[len(h):]
	}
	return p
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// ---- finding apps ----

type desktopFile struct{ path, id, name, exec, icon, wmclass, flatpak string }

func parseDesktop(path string) (d desktopFile, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return d, false
	}
	d.path, d.id = path, strings.TrimSuffix(filepath.Base(path), ".desktop")
	in, typ, hidden := false, "", false
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			in = l == "[Desktop Entry]"
			continue
		}
		k, v, found := strings.Cut(l, "=")
		if !in || !found {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Name":
			d.name = strings.TrimSpace(v)
		case "Exec":
			d.exec = v
		case "Icon":
			d.icon = v
		case "StartupWMClass":
			d.wmclass = v
		case "X-Flatpak":
			d.flatpak = v
		case "Type":
			typ = v
		case "NoDisplay", "Hidden":
			hidden = hidden || v == "true"
		}
	}
	return d, typ == "Application" && d.name != "" && !hidden
}

// execPath returns the program an Exec= line runs, skipping env, VAR=value and pkexec.
func execPath(cmdline string) string {
	for _, f := range execArgs(cmdline) {
		if f == "env" || f == "pkexec" || strings.Contains(f, "=") && !strings.HasPrefix(f, "/") {
			continue
		}
		return f
	}
	return ""
}

// execArgs splits an Exec= line the way the desktop entry spec quotes it:
// double quotes group words, and a backslash escapes the next character inside them.
func execArgs(cmdline string) []string {
	var args []string
	var cur strings.Builder
	quoted, started := false, false
	for i := 0; i < len(cmdline); i++ {
		switch c := cmdline[i]; {
		case c == '\\' && quoted && i+1 < len(cmdline):
			i++
			cur.WriteByte(cmdline[i])
		case c == '"':
			quoted, started = !quoted, true
		case (c == ' ' || c == '\t') && !quoted:
			if started || cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
		}
	}
	if started || cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func execBase(exec string) string { return filepath.Base(execPath(exec)) }

var desktopDirs = []string{
	"~/.local/share/applications", "~/.local/share/flatpak/exports/share/applications",
	"/var/lib/flatpak/exports/share/applications", "/var/lib/snapd/desktop/applications",
	"/usr/local/share/applications", "/usr/share/applications",
}

func listApps() []*App {
	seen := map[string]bool{}
	byID := map[string]*App{}
	var order []string
	var dpkgFiles []string
	pending := map[string][]desktopFile{} // desktop path → entry, resolved through dpkg -S
	add := func(id, source string, d desktopFile) {
		a := byID[id]
		if a == nil {
			a = &App{ID: id, Name: d.name, Icon: d.icon, Source: source, names: map[string]bool{}}
			byID[id] = a
			order = append(order, id)
		}
		a.Desktops = append(a.Desktops, d.path)
		if a.cmdline == "" {
			a.cmdline = d.exec
		}
		for _, n := range []string{d.id, lastDot(d.id), execBase(d.exec), d.wmclass, d.flatpak, lastDot(d.flatpak)} {
			if n = strings.ToLower(n); len(n) >= 3 && !genericNames[n] {
				a.names[n] = true
			}
		}
	}
	for _, dir := range desktopDirs {
		for _, p := range globs(dir + "/*.desktop") {
			base := filepath.Base(p)
			if seen[base] {
				continue // a user launcher overrides the system one
			}
			seen[base] = true
			d, ok := parseDesktop(p)
			if !ok {
				continue
			}
			switch {
			case strings.HasPrefix(p, "/var/lib/snapd/"):
				add("snap:"+strings.SplitN(d.id, "_", 2)[0], "snap", d)
			case strings.HasPrefix(p, "/var/lib/flatpak/"):
				add("flatpak:"+d.id, "flatpak", d)
			case strings.Contains(p, "/.local/share/flatpak/"):
				add("flatpak-user:"+d.id, "flatpak-user", d)
			case strings.HasPrefix(p, home()+"/"):
				if ex := execPath(d.exec); strings.Contains(strings.ToLower(ex), ".appimage") {
					add("appimage:"+ex, "appimage", d)
				} else {
					add("local:"+p, "local", d)
				}
			default:
				dpkgFiles = append(dpkgFiles, p)
				pending[p] = append(pending[p], d)
			}
		}
	}
	owners := dpkgOwners(dpkgFiles)
	for _, p := range dpkgFiles {
		for _, d := range pending[p] {
			if pkg := owners[p]; pkg != "" {
				add("apt:"+pkg, "apt", d)
			} else {
				add("local:"+p, "local", d)
			}
		}
	}

	var apts []string
	for _, id := range order {
		if pkg, ok := strings.CutPrefix(id, "apt:"); ok {
			apts = append(apts, pkg)
			byID[id].names[pkg] = true
		}
	}
	info := cmdOutAny("dpkg-query", append([]string{"-W", "-f=${Package}|${Priority}|${Essential}|${Installed-Size}\n"}, apts...)...)
	for _, l := range strings.Split(info, "\n") {
		f := strings.Split(l, "|")
		if len(f) != 4 {
			continue
		}
		a := byID["apt:"+f[0]]
		if a == nil {
			continue
		}
		var kb int64
		fmt.Sscan(f[3], &kb)
		a.Bytes = kb * 1024
		if f[2] == "yes" || f[1] == "required" || f[1] == "important" || matchesAny(f[0], protectedPkgs) {
			a.protected = "part of the system"
		}
	}
	snapRev := map[string]string{}
	for _, l := range strings.Split(cmdOut("snap", "list"), "\n") {
		if f := strings.Fields(l); len(f) >= 3 {
			snapRev[f[0]] = f[2]
		}
	}
	for _, id := range order {
		a := byID[id]
		kind, ref, _ := strings.Cut(id, ":")
		switch kind {
		case "apt":
			a.Installed = mtime("/var/lib/dpkg/info/"+ref+".list", "/var/lib/dpkg/info/"+ref+":amd64.list")
			a.LastUsed = atime(programPath(a.cmdline))
		case "snap":
			a.names[ref] = true
			if st, err := os.Stat("/var/lib/snapd/snaps/" + ref + "_" + snapRev[ref] + ".snap"); err == nil {
				a.Bytes = st.Size()
			}
			// /snap is a read-only mount, so its files never get a new access time; the app's data folder does change.
			a.Installed = mtime("/var/lib/snapd/snaps/" + ref + "_" + snapRev[ref] + ".snap")
			a.LastUsed = newestMtime(expand("~/snap/"+ref), 2)
			if matchesAny(ref, protectedSnaps) {
				a.protected = "part of the system"
			}
		case "flatpak":
			a.Bytes = diskUsage("/var/lib/flatpak/app/" + ref)
			a.Installed = mtime("/var/lib/flatpak/app/" + ref + "/current/active")
			a.LastUsed = newestMtime(expand("~/.var/app/"+ref), 2)
		case "flatpak-user":
			a.Bytes = diskUsage(expand("~/.local/share/flatpak/app/" + ref))
			a.Installed = mtime(expand("~/.local/share/flatpak/app/" + ref + "/current/active"))
			a.LastUsed = newestMtime(expand("~/.var/app/"+ref), 2)
		case "appimage":
			if st, err := os.Stat(ref); err == nil {
				a.Bytes = st.Size()
			}
			a.Installed, a.LastUsed = mtime(ref), atime(ref)
		case "local":
			if !strings.HasPrefix(ref, home()+"/") {
				a.protected = "installed outside your home folder; use its own uninstaller"
			}
			a.Installed, a.LastUsed = mtime(ref), atime(programPath(a.cmdline))
		}
		for n := range a.names {
			for _, b := range chromiumBrowsers {
				if n == strings.ToLower(filepath.Base(b.config)) || n == "firefox" {
					a.browser = true
				}
			}
		}
	}
	apps := make([]*App, 0, len(order))
	for _, id := range order {
		apps = append(apps, byID[id])
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })
	return apps
}

func lastDot(s string) string { return s[strings.LastIndex(s, ".")+1:] }

func sortApps(apps []*App, by string) {
	less := map[string]func(a, b *App) bool{
		"size":      func(a, b *App) bool { return a.Bytes > b.Bytes },
		"installed": func(a, b *App) bool { return a.Installed > b.Installed },
		"used":      func(a, b *App) bool { return a.LastUsed < b.LastUsed },
	}[by]
	if less != nil {
		sort.SliceStable(apps, func(i, j int) bool { return less(apps[i], apps[j]) })
	}
}

// programPath resolves the program an Exec= line starts.
func programPath(cmdline string) string {
	p := execPath(cmdline)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		var err error
		if p, err = exec.LookPath(p); err != nil {
			return ""
		}
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// atime is when a file was last read: with the default relatime mount option,
// launching a program updates it at most once a day, which is enough for "last used".
func atime(path string) int64 {
	var st syscall.Stat_t
	if path == "" || syscall.Stat(path, &st) != nil {
		return 0
	}
	return st.Atim.Sec
}

// mtime returns the modification time of the first path that exists.
func mtime(paths ...string) int64 {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			return st.ModTime().Unix()
		}
	}
	return 0
}

// newestMtime is the latest change inside dir, looking depth levels down.
func newestMtime(dir string, depth int) int64 {
	var t int64
	base := strings.Count(dir, "/")
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, e := d.Info(); e == nil && info.ModTime().Unix() > t {
			t = info.ModTime().Unix()
		}
		if d.IsDir() && strings.Count(p, "/")-base >= depth {
			return fs.SkipDir
		}
		return nil
	})
	return t
}

// ago describes a unix time relative to now: "today", "3 days ago", "never".
func ago(unix int64) string {
	if unix == 0 {
		return "never"
	}
	d := int(time.Since(time.Unix(unix, 0)).Hours() / 24)
	switch {
	case d < 1:
		return "today"
	case d == 1:
		return "yesterday"
	case d < 60:
		return fmt.Sprintf("%d days ago", d)
	case d < 730:
		return fmt.Sprintf("%d months ago", d/30)
	default:
		return fmt.Sprintf("%d years ago", d/365)
	}
}

// dpkgOwners maps each file to the package that installed it.
func dpkgOwners(files []string) map[string]string {
	out := map[string]string{}
	if len(files) == 0 {
		return out
	}
	for _, l := range strings.Split(cmdOutAny("dpkg", append([]string{"-S"}, files...)...), "\n") {
		i := strings.LastIndex(l, ": /")
		if i < 0 || strings.HasPrefix(l, "diversion") {
			continue
		}
		pkg := strings.TrimSpace(strings.Split(l[:i], ",")[0])
		out[l[i+2:]] = strings.SplitN(pkg, ":", 2)[0]
	}
	return out
}

// cmdOutAny returns stdout even when the command exits non-zero (dpkg -S with unknown files).
func cmdOutAny(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		return ""
	}
	return strings.TrimSpace(string(out))
}
