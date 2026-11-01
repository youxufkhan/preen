package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// fakeHome points $HOME and XDG dirs at a temp folder and creates the given files.
func fakeHome(t *testing.T, files ...string) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(h, ".local/state"))
	for _, f := range files {
		p := filepath.Join(h, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, make([]byte, 8192), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func TestCleanAppliesOnlyChosenItemsAndHonorsWhitelist(t *testing.T) {
	h := fakeHome(t, ".cache/someapp/blob", ".cache/keep/blob", ".cache/pip/wheel", ".local/share/Trash/files/old.iso")
	os.MkdirAll(filepath.Join(h, ".config/preen"), 0o755)
	os.WriteFile(filepath.Join(h, ".config/preen/whitelist"), []byte("~/.cache/keep\n"), 0o644)
	wl, err := loadWhitelist()
	if err != nil {
		t.Fatal(err)
	}
	items := cleanScan(wl)
	byID := map[string]*Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	for _, id := range []string{"app-cache", "pip", "trash"} {
		if byID[id] == nil {
			t.Fatalf("missing item %q", id)
		}
	}
	for _, e := range byID["app-cache"].Entries {
		if filepath.Base(e.Path) == "keep" || filepath.Base(e.Path) == "pip" {
			t.Fatalf("app-cache includes %s", e.Path)
		}
	}
	jsonOut = true
	defer func() { jsonOut = false }()
	if err := runPlan("clean", "t", items, &opts{yes: true, only: map[string]bool{"app-cache": true, "trash": true}}, false); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]bool{".cache/someapp": false, ".local/share/Trash/files/old.iso": false, ".cache/keep/blob": true, ".cache/pip/wheel": true} {
		if exists(filepath.Join(h, f)) != want {
			t.Errorf("%s exists=%v, want %v", f, !want, want)
		}
	}
	if !exists(filepath.Join(h, ".local/state/preen/operations.log")) {
		t.Error("operations log not written")
	}
}

func TestUnreadableWhitelistStops(t *testing.T) {
	h := fakeHome(t)
	os.MkdirAll(filepath.Join(h, ".config/preen/whitelist"), 0o755) // a directory cannot be read as a file
	if _, err := loadWhitelist(); err == nil {
		t.Fatal("expected an error for an unreadable whitelist")
	}
}

func TestFreeableIgnoresFilesLinkedFromOutside(t *testing.T) {
	h := fakeHome(t, "cache/blob", "solo/file")
	os.MkdirAll(filepath.Join(h, "project"), 0o755)
	if err := os.Link(filepath.Join(h, "cache/blob"), filepath.Join(h, "project/blob")); err != nil {
		t.Skip("hard links unsupported:", err)
	}
	if du, fr := diskUsage(filepath.Join(h, "cache")), freeable(filepath.Join(h, "cache")); fr >= du {
		t.Errorf("freeable %d should be below du %d when the file is linked elsewhere", fr, du)
	}
	if freeable(filepath.Join(h, "solo")) == 0 {
		t.Error("an unlinked file should be freeable")
	}
	if freeable(h) < 8192 {
		t.Error("both links inside the scan: the file should count")
	}
}

func TestPurgeFindsOnlyRealArtifacts(t *testing.T) {
	h := fakeHome(t,
		"p/web/package.json", "p/web/node_modules/x/index.js",
		"p/stray/node_modules/y.js", // no package.json: not a project
		"p/rs/Cargo.toml", "p/rs/target/debug/app",
		"p/py/.venv/pyvenv.cfg", "p/py/.venv/lib/z.py")
	got := map[string]bool{}
	for _, it := range purgeScan([]string{filepath.Join(h, "p")}) {
		got[it.ID] = true
	}
	for _, want := range []string{"p/web/node_modules", "p/rs/target", "p/py/.venv"} {
		if !got[filepath.Join(h, want)] {
			t.Errorf("missing %s", want)
		}
	}
	if got[filepath.Join(h, "p/stray/node_modules")] {
		t.Error("node_modules without package.json was treated as an artifact")
	}
}

func TestInstallerScan(t *testing.T) {
	h := fakeHome(t, "Downloads/app_1.0_amd64.deb", "Downloads/tool.AppImage", "Downloads/notes.txt")
	sel := map[string]bool{}
	for _, it := range installerScan() {
		sel[filepath.Base(it.ID)] = it.Selected
	}
	if !sel["app_1.0_amd64.deb"] || sel["tool.AppImage"] || len(sel) != 2 {
		t.Errorf("unexpected installer selection: %v (home %s)", sel, h)
	}
}

func TestDesktopParsing(t *testing.T) {
	h := fakeHome(t)
	p := filepath.Join(h, "x.desktop")
	os.WriteFile(p, []byte("[Desktop Entry]\nType=Application\nName=My App\nName[de]=Meine\nExec=env FOO=1 \"/opt/My App/run\" %U\nIcon=myapp\n[Desktop Action new]\nName=New\n"), 0o644)
	d, ok := parseDesktop(p)
	if !ok || d.name != "My App" || d.icon != "myapp" {
		t.Fatalf("parsed %+v ok=%v", d, ok)
	}
	for in, want := range map[string]string{
		`env FOO=1 /usr/bin/app --x`:             "/usr/bin/app",
		`"/opt/YouTube Music/youtube-music" %U`:  "/opt/YouTube Music/youtube-music",
		`pkexec /usr/bin/software-properties-qt`: "/usr/bin/software-properties-qt",
		`"/opt/a \"b\"/run"`:                     `/opt/a "b"/run`,
	} {
		if got := execPath(in); got != want {
			t.Errorf("execPath(%s) = %q, want %q", in, got, want)
		}
	}
	os.WriteFile(p, []byte("[Desktop Entry]\nType=Application\nName=Hidden\nNoDisplay=true\n"), 0o644)
	if _, ok := parseDesktop(p); ok {
		t.Error("NoDisplay entry should be skipped")
	}
}

func TestSmallParsers(t *testing.T) {
	if parseSize("512.5M") != 537395200 || parseSize("1G") != 1<<30 {
		t.Error("parseSize")
	}
	if lastField("Archived and active journals take up 196.8M in the file system.") != "196.8M" {
		t.Error("lastField")
	}
	if !verLess("7.0.0-34-generic", "7.0.0-38-generic") || verLess("7.0.10-1-generic", "7.0.9-1-generic") {
		t.Error("verLess")
	}
	if human(1_500_000) != "1.5 MB" || human(999) != "999 B" {
		t.Error("human:", human(1_500_000))
	}
}

func TestLockedBy(t *testing.T) {
	dir := t.TempDir()
	live, dead := filepath.Join(dir, "live"), filepath.Join(dir, "dead")
	os.Symlink("my-host-name-"+strconv.Itoa(os.Getpid()), live)
	os.Symlink("my-host-name-999999999", dead)
	if !lockedBy(live) || lockedBy(dead) || lockedBy(filepath.Join(dir, "none")) {
		t.Error("lockedBy")
	}
}

func TestPackageGuards(t *testing.T) {
	if !have("dpkg-query") {
		t.Skip("not a dpkg system")
	}
	for _, p := range []string{"bash", "coreutils", "", "../etc", "kubuntu-desktop",
		"btop+", "btop-", "bt*", "~nbtop", "?name(btop)", "btop:amd64", "-btop", "Btop", "a b"} {
		if checkRemovablePackage(p) == nil {
			t.Errorf("package %q passed the removal guard", p)
		}
	}
	for _, s := range []string{"core22", "snapd", "bare", "gnome-46-2404", "a/b", "fire*", "firefox-", "-firefox"} {
		if checkRemovableSnap(s) == nil {
			t.Errorf("snap %q passed the removal guard", s)
		}
	}
}
