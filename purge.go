package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rebuildable project artifacts and the file that proves the folder is one.
var artifacts = map[string][]string{
	"node_modules":  {"package.json"},
	"target":        {"Cargo.toml", "pom.xml"},
	"build":         {"build.gradle", "build.gradle.kts", "CMakeLists.txt", "package.json", "pubspec.yaml", "pyproject.toml", "setup.py"},
	"dist":          {"package.json", "pyproject.toml", "setup.py"},
	".venv":         {"pyvenv.cfg@"}, // @: marker lives inside the artifact
	"venv":          {"pyvenv.cfg@"},
	".gradle":       {"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"},
	".next":         {"package.json"},
	".nuxt":         {"package.json"},
	".svelte-kit":   {"package.json"},
	".turbo":        {"package.json"},
	".parcel-cache": {"package.json"},
	".angular":      {"package.json"},
	".dart_tool":    {"pubspec.yaml"},
	"Pods":          {"Podfile"},
	".zig-cache":    {"build.zig"},
	"zig-out":       {"build.zig"},
}

var defaultProjectRoots = []string{
	"~/Projects", "~/projects", "~/dev", "~/Developer", "~/code", "~/Code", "~/src", "~/workspace",
	"~/work", "~/GitHub", "~/github", "~/git", "~/repos", "~/apps",
}

const purgeDepth = 6

func cmdPurge(args []string) error {
	fs, o := planFlags("purge")
	fs.Parse(args)
	roots := projectRoots()
	if fs.NArg() > 0 {
		roots = fs.Args()
	}
	return runPlan("purge", "Purge project build artifacts", purgeScan(roots), o, false)
}

// projectRoots reads ~/.config/preen/purge_paths (one folder per line) or uses the defaults.
func projectRoots() []string {
	var roots []string
	if b, err := os.ReadFile(filepath.Join(configDir(), "purge_paths")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				roots = append(roots, expand(l))
			}
		}
		return roots
	}
	for _, r := range defaultProjectRoots {
		if st, err := os.Stat(expand(r)); err == nil && st.IsDir() {
			roots = append(roots, expand(r))
		}
	}
	return roots
}

func purgeScan(roots []string) []*Item {
	var found []string
	for _, root := range roots {
		root = filepath.Clean(expand(root))
		progress("Scanning " + tildePath(root))
		depth0 := strings.Count(root, "/")
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if strings.Count(p, "/")-depth0 > purgeDepth {
				return fs.SkipDir
			}
			name := d.Name()
			if markers, ok := artifacts[name]; ok && p != root && isArtifact(p, markers) {
				found = append(found, p)
				return fs.SkipDir
			}
			if name == "node_modules" || name == ".git" || (strings.HasPrefix(name, ".") && p != root) {
				return fs.SkipDir
			}
			return nil
		})
	}
	es := entries(found)
	week := time.Now().AddDate(0, 0, -7)
	var items []*Item
	for _, e := range es {
		project := filepath.Dir(e.Path)
		it := &Item{ID: e.Path, Group: "Projects", Label: tildePath(project) + " · " + filepath.Base(e.Path),
			Bytes: e.Bytes, Risk: "safe", Selected: true, Entries: []Entry{e}}
		if gitTracked(project, filepath.Base(e.Path)) {
			continue // committed to git: not a build artifact here
		}
		if newest(e.Path).After(week) {
			it.Selected, it.Note = false, "used in the last 7 days"
		}
		items = append(items, it)
	}
	clearProgress()
	return items
}

func isArtifact(p string, markers []string) bool {
	for _, m := range markers {
		if inside, ok := strings.CutSuffix(m, "@"); ok {
			if exists(filepath.Join(p, inside)) {
				return true
			}
		} else if exists(filepath.Join(filepath.Dir(p), m)) {
			return true
		}
	}
	return false
}

func gitTracked(project, name string) bool {
	return cmdOut("git", "-C", project, "ls-files", "--", name+"/") != ""
}

// newest returns the latest modification time of the folder itself or its direct entries.
// ponytail: checks one level only; a full walk doubles scan time on big node_modules.
func newest(dir string) time.Time {
	var t time.Time
	if st, err := os.Stat(dir); err == nil {
		t = st.ModTime()
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	return t
}
