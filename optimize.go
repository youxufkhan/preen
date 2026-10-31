package main

import (
	"fmt"
	"os"
	"strings"
)

func cmdOptimize(args []string) error {
	fs, o := planFlags("optimize")
	fs.Parse(args)
	progress("Checking system health")
	items := append(maintenanceTasks(), diagnose()...)
	clearProgress()
	return runPlan("optimize", "Maintenance", items, o, false)
}

// maintenanceTasks are bounded refresh jobs; each says why it is skipped when it is.
func maintenanceTasks() []*Item {
	var items []*Item
	task := func(id, label, note string, ok bool, why string, fn func() error) {
		it := &Item{ID: id, Group: "Maintenance", Label: label, Risk: "safe", Selected: ok, Note: note, apply: fn}
		if !ok {
			it.Disabled, it.Note = true, why
		}
		items = append(items, it)
	}
	userApps := expand("~/.local/share/applications")
	task("font-cache", "Rebuild font cache", "fixes missing or stale fonts", have("fc-cache"), "fc-cache not installed",
		func() error { return run("fc-cache", "-f") })
	task("kde-menu-cache", "Rebuild KDE app menu cache", "fixes missing or stale launchers", have("kbuildsycoca6"), "not a KDE session",
		func() error { return run("kbuildsycoca6", "--noincremental") })
	task("desktop-db", "Refresh file associations", "update-desktop-database for your launchers", have("update-desktop-database") && exists(userApps),
		"nothing to refresh", func() error { return run("update-desktop-database", userApps) })

	root := func(id, label, note string, ok bool, why string) {
		task(id, label, note, ok, why, nil)
		it := items[len(items)-1]
		it.Root, it.action = true, id
	}
	root("dns-flush", "Flush DNS cache", "systemd-resolved", have("resolvectl"), "systemd-resolved not in use")
	trimTimer := cmdOut("systemctl", "is-enabled", "fstrim.timer") == "enabled"
	root("fstrim", "Trim SSDs", "tells the SSD which blocks are free", have("fstrim") && !trimTimer, "fstrim.timer already trims weekly")
	retain := cmdOut("snap", "get", "system", "refresh.retain")
	root("snap-retain", "Keep 2 versions of each snap", "snap keeps 3 by default; old versions use disk space",
		have("snap") && retain != "2", "already set")
	return items
}

// diagnose returns read-only findings. They are never applied.
func diagnose() []*Item {
	var items []*Item
	find := func(label, note string) {
		items = append(items, &Item{ID: "health:" + label, Group: "Health", Label: label, Risk: "info", Disabled: true, Note: note, Icon: "data-warning"})
	}
	failed := strings.Fields(cmdOut("systemctl", "--failed", "--no-legend", "--plain", "--quiet"))
	failed = append(failed, strings.Fields(cmdOut("systemctl", "--user", "--failed", "--no-legend", "--plain", "--quiet"))...)
	var units []string
	for _, f := range failed {
		if strings.Contains(f, ".") && (strings.HasSuffix(f, ".service") || strings.HasSuffix(f, ".mount") || strings.HasSuffix(f, ".timer") || strings.HasSuffix(f, ".socket")) {
			units = append(units, f)
		}
	}
	if len(units) > 0 {
		find(fmt.Sprintf("%d failed services", len(units)), strings.Join(units, ", ")+" · check: systemctl status <name>")
	}
	if total, free := diskSpace("/"); total > 0 && free*10 < total {
		find("Root disk almost full", fmt.Sprintf("%s free of %s · run Clean", human(free), human(total)))
	}
	if p := psi("memory"); p > 10 {
		find("Memory pressure", fmt.Sprintf("apps waited on memory %.0f%% of the last 10 s · close heavy apps or add swap", p))
	}
	if b := diskUsage(expand("~/.local/share/baloo")); b > 2e9 {
		find("Large file search index", human(b)+" in ~/.local/share/baloo · balooctl6 purge rebuilds it smaller")
	}
	if len(items) == 0 {
		find("No problems found", "checked failed services, disk space, memory pressure and search index")
		items[0].Icon = "checkmark"
	}
	return items
}

func diskSpace(path string) (total, free int64) {
	st, err := statfs(path)
	if err != nil {
		return 0, 0
	}
	return int64(st.Blocks) * st.Bsize, int64(st.Bavail) * st.Bsize
}

// psi reads "some avg10" from /proc/pressure/<kind>.
func psi(kind string) float64 {
	b, err := os.ReadFile("/proc/pressure/" + kind)
	if err != nil {
		return 0
	}
	var v float64
	for _, f := range strings.Fields(string(b)) {
		if x, ok := strings.CutPrefix(f, "avg10="); ok {
			fmt.Sscan(x, &v)
			return v
		}
	}
	return 0
}
