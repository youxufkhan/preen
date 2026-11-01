package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func cmdClean(args []string) error {
	fs, o := planFlags("clean")
	fs.Parse(args)
	wl, err := loadWhitelist()
	if err != nil {
		return err
	}
	return runPlan("clean", "Clean your system", cleanScan(wl), o, false)
}

// Chromium-family browsers: ~/.config/<config> holds profiles, ~/.cache/<cache> the disk cache.
var chromiumBrowsers = []struct{ id, label, config, cache string }{
	{"chrome", "Google Chrome", "google-chrome", "google-chrome"},
	{"chrome-beta", "Google Chrome Beta", "google-chrome-beta", "google-chrome-beta"},
	{"chromium", "Chromium", "chromium", "chromium"},
	{"brave", "Brave", "BraveSoftware/Brave-Browser", "BraveSoftware/Brave-Browser"},
	{"edge", "Microsoft Edge", "microsoft-edge", "microsoft-edge"},
	{"vivaldi", "Vivaldi", "vivaldi", "vivaldi"},
	{"opera", "Opera", "opera", "opera"},
}

// Cache folders inside a Chromium or Electron profile. Never cookies, history or logins.
var chromiumCacheDirs = []string{
	"Cache", "Code Cache", "GPUCache", "DawnCache", "DawnGraphiteCache", "DawnWebGPUCache",
	"Service Worker/CacheStorage", "Service Worker/ScriptCache", "CachedData", "CachedExtensionVSIXs",
}

// ~/.cache entries owned by a specific item (or never touched), excluded from "App caches".
var claimedCache = []string{
	"thumbnails", "google-chrome*", "chromium", "BraveSoftware", "microsoft-edge*", "vivaldi", "opera",
	"mozilla", "yarn", "pnpm", "pip", "uv", "go-build", "ms-playwright*", "puppeteer", "huggingface", "JetBrains",
	"ksycoca*", "codex-runtimes",
}

func cleanScan(wl []string) []*Item {
	var jobs []func() *Item
	job := func(f func() *Item) { jobs = append(jobs, f) }
	paths := func(id, group, label, risk string, ps []string) *Item {
		var keep []string
		for _, p := range ps {
			if !whitelisted(wl, p) {
				keep = append(keep, p)
			}
		}
		es := entries(keep)
		if len(es) == 0 {
			return nil
		}
		return &Item{ID: id, Group: group, Label: label, Risk: risk, Selected: risk == "safe", Entries: es, Bytes: sum(es)}
	}
	busy := func(it *Item, isBusy bool, who string) *Item {
		if it != nil && isBusy {
			it.Disabled, it.Selected, it.Note = true, false, "close "+who+" first"
		}
		return it
	}

	job(func() *Item { return rootItem(aptCacheItem()) })
	job(func() *Item { return rootItem(journalItem()) })
	job(func() *Item { return rootItem(snapDisabledItem()) })
	job(func() *Item {
		return rootItem(paths("crash-dumps", "System", "Crash reports and core dumps", "safe",
			append(children("/var/crash"), children("/var/lib/systemd/coredump")...)), "crash-dumps")
	})
	job(func() *Item { return rootItem(oldKernelsItem()) })
	job(func() *Item { return rootItem(autoremoveItem()) })
	job(func() *Item { return rootItem(rcConfigsItem()) })

	job(func() *Item {
		var trash []string
		for _, d := range append([]string{"~/.local/share/Trash"}, globs("/media/*/*/.Trash-"+strconv.Itoa(os.Getuid()))...) {
			for _, sub := range []string{"files", "info", "expunged"} {
				trash = append(trash, children(filepath.Join(d, sub))...)
			}
		}
		return paths("trash", "Trash", "Trash", "safe", trash)
	})

	for _, b := range chromiumBrowsers {
		cfg := expand("~/.config/" + b.config)
		job(func() *Item {
			ps := children("~/.cache/" + b.cache)
			for _, d := range chromiumCacheDirs {
				ps = append(ps, globs(cfg+"/*/"+d)...)
			}
			ps = append(ps, globs(cfg+"/GrShaderCache", cfg+"/ShaderCache", cfg+"/GraphiteDawnCache", cfg+"/component_crx_cache")...)
			return busy(paths(b.id, "Browsers", b.label+" cache", "safe", ps), lockedBy(cfg+"/SingletonLock"), b.label)
		})
		job(func() *Item {
			return busy(paths(b.id+"-ai-model", "Browsers", b.label+" on-device AI model", "review", globs(cfg+"/OptGuideOnDevice*Model")),
				lockedBy(cfg+"/SingletonLock"), b.label)
		})
	}
	job(func() *Item {
		return busy(paths("firefox", "Browsers", "Firefox cache", "safe",
			globs("~/.cache/mozilla/firefox/*/cache2", "~/.cache/mozilla/firefox/*/startupCache", "~/.cache/mozilla/firefox/*/thumbnails",
				"~/snap/firefox/common/.cache/mozilla/firefox/*/cache2")), running("firefox", "firefox-bin", "firefox-esr"), "Firefox")
	})

	known := map[string]bool{}
	for _, b := range chromiumBrowsers {
		known[strings.SplitN(b.config, "/", 2)[0]] = true
	}
	for _, dir := range globs("~/.config/*") {
		name := filepath.Base(dir)
		if known[name] || !isElectron(dir) {
			continue
		}
		job(func() *Item {
			var ps []string
			for _, d := range chromiumCacheDirs {
				ps = append(ps, globs(dir+"/"+d)...)
			}
			label := prettyName(name)
			return busy(paths("electron:"+name, "Apps", label+" cache", "safe", ps), lockedBy(dir+"/SingletonLock"), label)
		})
	}
	job(func() *Item {
		var generic []string
		for _, p := range children("~/.cache") {
			if !matchesAny(filepath.Base(p), claimedCache) {
				generic = append(generic, p)
			}
		}
		return paths("app-cache", "Apps", "App caches (~/.cache)", "safe", generic)
	})
	job(func() *Item {
		return paths("thumbnails", "Apps", "Thumbnails", "safe", children("~/.cache/thumbnails"))
	})
	job(func() *Item {
		return paths("flatpak-cache", "Apps", "Flatpak app caches", "safe", globs("~/.var/app/*/cache/*"))
	})
	job(func() *Item {
		return paths("snap-app-cache", "Apps", "Snap app caches", "safe", globs("~/snap/*/common/.cache/*"))
	})

	gomod := os.Getenv("GOMODCACHE")
	if gomod == "" {
		gomod = "~/go/pkg/mod"
	}
	dev := []struct {
		id, label, risk string
		ps              []string
	}{
		{"npm", "npm cache", "safe", globs("~/.npm/_cacache", "~/.npm/_logs", "~/.npm/_npx")},
		{"pnpm", "pnpm cache", "safe", globs("~/.cache/pnpm")},
		{"yarn", "Yarn cache", "safe", globs("~/.cache/yarn", "~/.yarn/berry/cache")},
		{"bun", "Bun cache", "safe", globs("~/.bun/install/cache")},
		{"pip", "pip cache", "safe", globs("~/.cache/pip")},
		{"uv", "uv cache", "safe", globs("~/.cache/uv")},
		{"go-build", "Go build cache", "safe", globs("~/.cache/go-build")},
		{"go-mod", "Go module cache", "review", globs(gomod)},
		{"cargo", "Cargo registry cache", "safe", globs("~/.cargo/registry/cache", "~/.cargo/registry/src", "~/.cargo/git/checkouts")},
		{"gradle", "Gradle caches", "safe", globs("~/.gradle/caches", "~/.gradle/wrapper/dists")},
		{"maven", "Maven repository", "review", globs("~/.m2/repository")},
		{"jetbrains-logs", "JetBrains logs", "safe", globs("~/.cache/JetBrains/*/log")},
		{"playwright", "Playwright browsers", "review", globs("~/.cache/ms-playwright*")},
		{"puppeteer", "Puppeteer browsers", "review", globs("~/.cache/puppeteer")},
		{"huggingface", "Hugging Face models", "review", globs("~/.cache/huggingface")},
	}
	for _, d := range dev {
		job(func() *Item {
			it := paths(d.id, "Developer", d.label, d.risk, d.ps)
			if d.id == "gradle" && it != nil && runningCmdline("GradleDaemon") {
				it.Disabled, it.Selected, it.Note = true, false, "a Gradle daemon is running"
			}
			return it
		})
	}

	res := make([]*Item, len(jobs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	done, found := 0, int64(0)
	progressN(fmt.Sprintf("Checking %d locations", len(jobs)), 0, len(jobs), 0, 0)
	sem := make(chan struct{}, 6)
	for i, f := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			it := f()
			res[i] = it
			mu.Lock()
			defer mu.Unlock()
			done++
			msg := "" // nothing here; keep showing the last location measured
			if it != nil {
				found += it.Bytes
				msg = "Measured " + it.Label + " · " + human(it.Bytes)
			}
			progressN(msg, done, len(jobs), found, 0)
		}()
	}
	wg.Wait()
	clearProgress()

	ids := map[string]bool{}
	for _, w := range wl {
		if !strings.Contains(w, "/") {
			ids[w] = true
		}
	}
	var items []*Item
	for _, it := range res {
		if it != nil && !ids[it.ID] {
			items = append(items, it)
		}
	}
	return items
}

// prettyName turns a config folder name into a display name.
func prettyName(dir string) string {
	if n, ok := map[string]string{"Code": "VS Code", "teams-for-linux": "Teams"}[dir]; ok {
		return n
	}
	return strings.ToUpper(dir[:1]) + dir[1:]
}

// rootItem marks an item as needing the privileged helper.
func rootItem(it *Item, action ...string) *Item {
	if it == nil {
		return nil
	}
	it.Root = true
	if len(action) > 0 {
		it.action = action[0]
	}
	if it.action == "" {
		it.action = it.ID
	}
	return it
}

// isElectron spots Chromium-based app profiles by their cache folders.
func isElectron(dir string) bool {
	for _, d := range []string{"Code Cache", "GPUCache"} {
		if st, err := os.Stat(filepath.Join(dir, d)); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

func runningCmdline(substr string) bool {
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue
		}
		if b, err := os.ReadFile("/proc/" + p.Name() + "/cmdline"); err == nil && strings.Contains(string(b), substr) {
			return true
		}
	}
	return false
}

// ---- system items (applied by the root helper) ----

func aptCacheItem() *Item {
	if !have("apt-get") {
		return nil
	}
	es := entries(globs("/var/cache/apt/archives/*.deb", "/var/cache/apt/archives/partial/*", "/var/cache/apt/*.bin"))
	if len(es) == 0 {
		return nil
	}
	return &Item{ID: "apt-cache", Group: "System", Label: "APT package cache", Risk: "safe", Selected: true, Entries: es, Bytes: sum(es), action: "apt-clean"}
}

const journalKeep = 200 << 20

func journalItem() *Item {
	used := parseSize(lastField(cmdOut("journalctl", "--disk-usage")))
	if used <= journalKeep+50<<20 {
		return nil
	}
	return &Item{ID: "journal", Group: "System", Label: "System logs (keep newest 200 MB)", Risk: "safe", Selected: true,
		Bytes: used - journalKeep, Entries: []Entry{{"/var/log/journal", used}}, action: "journal-vacuum"}
}

// parseSize reads journalctl/systemd sizes like "901.1M" or "1.2G" (binary units).
func parseSize(s string) int64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "B")
	if s == "" {
		return 0
	}
	mult := map[byte]float64{'K': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40}[s[len(s)-1]]
	if mult == 0 {
		mult = 1
	} else {
		s = s[:len(s)-1]
	}
	f, _ := strconv.ParseFloat(s, 64)
	return int64(f * mult)
}

// lastField returns the size token from "Archived and active journals take up 901.1M in the file system."
func lastField(s string) string {
	f := strings.Fields(s)
	for i, w := range f {
		if w == "up" && i+1 < len(f) {
			return f[i+1]
		}
	}
	return ""
}

func snapDisabled() [][2]string {
	var out [][2]string
	for _, l := range strings.Split(cmdOut("snap", "list", "--all"), "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && strings.Contains(l, "disabled") {
			out = append(out, [2]string{f[0], f[2]})
		}
	}
	return out
}

func snapDisabledItem() *Item {
	if !have("snap") {
		return nil
	}
	var ps []string
	for _, s := range snapDisabled() {
		ps = append(ps, "/var/lib/snapd/snaps/"+s[0]+"_"+s[1]+".snap")
	}
	es := entries(ps)
	if len(es) == 0 {
		return nil
	}
	return &Item{ID: "snap-disabled", Group: "System", Label: "Old snap versions", Risk: "safe", Selected: true, Entries: es, Bytes: sum(es)}
}

// oldKernels returns installed kernel versions other than the running and the newest one,
// with every installed package that belongs to them.
func oldKernels() (vers, pkgs []string) {
	runningK := cmdOut("uname", "-r")
	var installed []string
	all := map[string]bool{}
	for _, l := range strings.Split(cmdOut("dpkg-query", "-W", "-f=${db:Status-Abbrev} ${Package}\n"), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || f[0] != "ii" {
			continue
		}
		all[f[1]] = true
		if v, ok := strings.CutPrefix(f[1], "linux-image-"); ok && v != "" && v[0] >= '0' && v[0] <= '9' {
			installed = append(installed, v)
		}
	}
	if len(installed) < 2 {
		return nil, nil
	}
	newest := installed[0]
	for _, v := range installed {
		if verLess(newest, v) {
			newest = v
		}
	}
	for _, v := range installed {
		if v == runningK || v == newest {
			continue
		}
		vers = append(vers, v)
		base := v
		if i := strings.LastIndex(v, "-"); i > 0 && (v[i+1] < '0' || v[i+1] > '9') {
			base = v[:i] // 7.0.0-34-generic → 7.0.0-34
		}
		for p := range all {
			if strings.HasPrefix(p, "linux-") && (strings.HasSuffix(p, "-"+v) || p == "linux-headers-"+base) {
				pkgs = append(pkgs, p)
			}
		}
	}
	return vers, pkgs
}

// verLess compares versions like "7.0.0-34-generic" by their numeric parts.
func verLess(a, b string) bool {
	num := func(s string) []int {
		var out []int
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
			n, _ := strconv.Atoi(f)
			out = append(out, n)
		}
		return out
	}
	x, y := num(a), num(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

func oldKernelsItem() *Item {
	vers, _ := oldKernels()
	var ps []string
	for _, v := range vers {
		ps = append(ps, globs("/lib/modules/"+v, "/boot/*-"+v, "/usr/src/linux-headers-"+strings.TrimSuffix(v, "-generic")+"*")...)
	}
	es := entries(ps)
	if len(es) == 0 {
		return nil
	}
	return &Item{ID: "old-kernels", Group: "System", Label: "Old kernels (" + strings.Join(vers, ", ") + ")", Risk: "review",
		Note: "keeps the running and newest kernel", Entries: es, Bytes: sum(es)}
}

func autoremovable() []string {
	var out []string
	for _, l := range strings.Split(cmdOut("apt-get", "-s", "autoremove"), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && f[0] == "Remv" {
			out = append(out, f[1])
		}
	}
	return out
}

func autoremoveItem() *Item {
	if !have("apt-get") {
		return nil
	}
	es := pkgEntries(autoremovable())
	if len(es) == 0 {
		return nil
	}
	return &Item{ID: "autoremove", Group: "System", Label: "Unused dependency packages", Risk: "review", Entries: es, Bytes: sum(es)}
}

func rcPackages() []string {
	var out []string
	for _, l := range strings.Split(cmdOut("dpkg-query", "-W", "-f=${db:Status-Abbrev} ${Package}\n"), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "rc" {
			out = append(out, f[1])
		}
	}
	return out
}

func rcConfigsItem() *Item {
	pk := rcPackages()
	if len(pk) == 0 {
		return nil
	}
	es := make([]Entry, len(pk))
	for i, p := range pk {
		es[i] = Entry{Path: p}
	}
	return &Item{ID: "rc-configs", Group: "System", Label: "Settings left by removed packages", Risk: "review",
		Note: strconv.Itoa(len(pk)) + " packages, config files only", Entries: es}
}

// pkgEntries lists packages with their installed size.
func pkgEntries(pkgs []string) []Entry {
	if len(pkgs) == 0 {
		return nil
	}
	out := cmdOut("dpkg-query", append([]string{"-W", "-f=${Package} ${Installed-Size}\n"}, pkgs...)...)
	var es []Entry
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) == 2 {
			kb, _ := strconv.ParseInt(f[1], 10, 64)
			es = append(es, Entry{Path: strings.SplitN(f[0], ":", 2)[0], Bytes: kb * 1024})
		}
	}
	return es
}
