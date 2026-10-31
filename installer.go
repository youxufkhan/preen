package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Installer file types. AppImages and ISOs may be in use, so they start unselected.
var installerExts = map[string]string{
	".deb": "safe", ".rpm": "safe", ".run": "safe", ".flatpakref": "safe", ".snap": "safe",
	".exe": "safe", ".msi": "safe", ".dmg": "safe", ".pkg": "safe",
	".appimage": "review", ".iso": "review",
}

func cmdInstaller(args []string) error {
	fs, o := planFlags("installer")
	fs.Parse(args)
	return runPlan("installer", "Remove installer files", installerScan(), o, false)
}

func installerScan() []*Item {
	progress("Looking for installer files")
	inUse := map[string]bool{} // AppImages that a launcher points to are installed apps
	for _, dir := range desktopDirs {
		for _, p := range globs(dir + "/*.desktop") {
			if d, ok := parseDesktop(p); ok {
				inUse[execPath(d.exec)] = true
			}
		}
	}
	var files []string
	for _, dir := range []string{"~/Downloads", "~/Desktop"} {
		files = append(files, globs(dir+"/*", dir+"/*/*")...)
	}
	var items []*Item
	for _, p := range files {
		risk, ok := installerExts[strings.ToLower(filepath.Ext(p))]
		st, err := os.Lstat(p)
		if !ok || err != nil || !st.Mode().IsRegular() || inUse[p] {
			continue
		}
		it := &Item{ID: p, Group: "Installers", Label: filepath.Base(p), Bytes: st.Size(), Risk: risk, Selected: risk == "safe",
			Note: tildePath(filepath.Dir(p)) + " · " + age(st.ModTime()), Entries: []Entry{{Path: p, Bytes: st.Size()}}}
		items = append(items, it)
	}
	clearProgress()
	return items
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd old", int(d.Hours()/24))
	case d < 2*365*24*time.Hour:
		return fmt.Sprintf("%dmo old", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy old", int(d.Hours()/24/365))
	}
}
