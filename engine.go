package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"preen/internal/safe"
)

// Item is one reviewable thing a command can remove or change.
type Item struct {
	ID       string  `json:"id"`
	Group    string  `json:"group"`
	Label    string  `json:"label"`
	Bytes    int64   `json:"bytes"`
	Risk     string  `json:"risk"` // safe | review | personal
	Selected bool    `json:"selected"`
	Root     bool    `json:"root,omitempty"`
	Note     string  `json:"note,omitempty"`     // extra context, or why it is disabled
	Disabled bool    `json:"disabled,omitempty"` // cannot be applied right now (e.g. app running)
	Entries  []Entry `json:"entries,omitempty"`
	Icon     string  `json:"icon,omitempty"`

	action   string       // helper action for root items
	required bool         // if this item fails, apply nothing after it
	apply    func() error // user-level items; nil means remove Entries permanently
}

// Entry is one path inside an Item, shown on the review screen.
type Entry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Event is one NDJSON line on stdout in --json mode. The GUI reads only these.
type Event struct {
	Type    string `json:"type"` // progress | item | result | done | error
	Message string `json:"message,omitempty"`
	Item    *Item  `json:"item,omitempty"`
	ID      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"` // applied | skipped | failed | unchanged | unavailable
	Bytes   int64  `json:"bytes,omitempty"`
	Count   int    `json:"count,omitempty"`
	Free    int64  `json:"free,omitempty"`
	Data    any    `json:"data,omitempty"` // analyze and status payloads
}

var (
	jsonOut bool
	outMu   sync.Mutex
)

func emit(e Event) {
	if !jsonOut {
		return
	}
	outMu.Lock()
	defer outMu.Unlock()
	b, _ := json.Marshal(e)
	os.Stdout.Write(append(b, '\n'))
}

// say prints human output; silent in --json mode.
func say(format string, a ...any) {
	if !jsonOut {
		fmt.Printf(format+"\n", a...)
	}
}

func progress(msg string) {
	emit(Event{Type: "progress", Message: msg})
	if !jsonOut && isTTY(os.Stderr) {
		fmt.Fprintf(os.Stderr, "\r\033[K%s", msg)
	}
}

func clearProgress() {
	if !jsonOut && isTTY(os.Stderr) {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// color wraps s in an ANSI color when stdout is a terminal.
func color(code, s string) string {
	if jsonOut || !isTTY(os.Stdout) || os.Getenv("NO_COLOR") != "" {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// opts are the flags shared by every scan → review → apply command.
type opts struct {
	dryRun, yes, all bool
	only             map[string]bool
}

func planFlags(name string) (*flag.FlagSet, *opts) {
	o := &opts{}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.BoolVar(&jsonOut, "json", false, "print NDJSON events")
	fs.BoolVar(&o.dryRun, "dry-run", false, "show what would be removed, change nothing")
	fs.BoolVar(&o.yes, "yes", false, "do not ask for confirmation")
	fs.BoolVar(&o.all, "all", false, "also select items marked review")
	fs.Func("only", "item ids to apply, comma-separated or repeated (overrides default selection)", func(s string) error {
		if o.only == nil {
			o.only = map[string]bool{}
		}
		if strings.HasPrefix(s, "/") {
			o.only[s] = true
			return nil
		}
		for _, id := range strings.Split(s, ",") {
			if id = strings.TrimSpace(id); id != "" {
				o.only[id] = true
			}
		}
		return nil
	})
	return fs, o
}

// runPlan shows items, asks, applies the selected ones, logs and reports.
func runPlan(cmd, title string, items []*Item, o *opts, trash bool) error {
	for _, it := range items {
		switch {
		case it.Disabled:
			it.Selected = false
		case o.only != nil:
			it.Selected = o.only[it.ID]
		case o.all:
			it.Selected = true
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Group != items[j].Group {
			return groupRank(items[i].Group) < groupRank(items[j].Group)
		}
		return items[i].Bytes > items[j].Bytes
	})

	var total int64
	var n int
	for _, it := range items {
		emit(Event{Type: "item", Item: it})
		if it.Selected {
			total += it.Bytes
			n++
		}
	}
	printItems(title, items)
	if len(items) == 0 {
		say("Nothing found.")
	}
	if o.dryRun || n == 0 {
		emit(Event{Type: "done", Bytes: total, Count: n, Free: freeBytes("/")})
		if !jsonOut && n > 0 {
			freed := ""
			if total > 0 {
				freed = ", about " + human(total)
			}
			say("\nDry run: %s selected%s. Nothing was changed.", plural(n, "item"), freed)
		}
		return nil
	}
	if !o.yes {
		if jsonOut || !isTTY(os.Stdin) {
			return errors.New("refusing to change anything without --yes in non-interactive mode")
		}
		fmt.Printf("\nRemove %s in %d items? [y/N] ", human(total), n)
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			say("Cancelled. Nothing was changed.")
			return nil
		}
	}

	run := newRun(cmd)
	var freed int64
	var done int
	ok := func(it *Item, err error) bool {
		if record(run, it, err) {
			freed += it.Bytes
			done++
			return true
		}
		return !it.required
	}
	var rootItems []*Item
	for _, it := range items {
		if it.Selected && it.Root {
			rootItems = append(rootItems, it)
		}
	}
	proceed := true
	if len(rootItems) > 0 {
		var actions []string
		for _, it := range rootItems {
			actions = append(actions, it.action)
		}
		progress("Waiting for administrator authentication")
		results, err := runHelper(actions)
		for _, it := range rootItems {
			r, got := results[it.action]
			switch {
			case got && r.Status == "applied":
				proceed = ok(it, nil) && proceed
			case got:
				proceed = ok(it, errors.New(r.Message)) && proceed
			case err != nil:
				proceed = ok(it, err) && proceed
			default:
				proceed = ok(it, errors.New("no result from helper")) && proceed
			}
		}
	}
	for _, it := range items {
		if !it.Selected || it.Root {
			continue
		}
		if !proceed {
			emit(Event{Type: "result", ID: it.ID, Status: "skipped", Message: "an earlier required step failed"})
			continue
		}
		progress("Working on " + it.Label)
		proceed = ok(it, applyItem(it, trash, run))
	}
	clearProgress()
	emit(Event{Type: "done", Bytes: freed, Count: done, Free: freeBytes("/")})
	run.log(oplogEntry{Action: "end", Bytes: freed})
	say("\n%s %s, %s freed. %s", color("1;32", "Done."), plural(done, "item"), human(freed), freeSummary())
	return nil
}

// applyItem removes an item's entries (or runs its apply func).
func applyItem(it *Item, trash bool, run *opRun) error {
	if it.apply != nil {
		return it.apply()
	}
	var errs []error
	for _, e := range it.Entries {
		var err error
		if trash {
			err = safe.Trash(e.Path)
		} else {
			err = forceRemove(e.Path)
		}
		run.log(oplogEntry{Action: map[bool]string{true: "trash", false: "remove"}[trash], Path: e.Path, Bytes: e.Bytes, Err: errString(err)})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// record emits and logs an item result; reports whether it succeeded.
func record(run *opRun, it *Item, err error) bool {
	if err != nil {
		emit(Event{Type: "result", ID: it.ID, Status: "failed", Message: err.Error()})
		run.log(oplogEntry{Action: "item", ID: it.ID, Err: err.Error()})
		say("  %s %s: %v", color("31", "✗"), it.Label, err)
		return false
	}
	emit(Event{Type: "result", ID: it.ID, Status: "applied", Bytes: it.Bytes})
	run.log(oplogEntry{Action: "item", ID: it.ID, Bytes: it.Bytes})
	say("  %s %s  %s", color("32", "✓"), it.Label, human(it.Bytes))
	return true
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var groups = []string{"System", "Trash", "Browsers", "Apps", "Developer", "Projects", "Installers", "Leftovers"}

func groupRank(g string) int {
	for i, x := range groups {
		if x == g {
			return i
		}
	}
	return len(groups)
}

func printItems(title string, items []*Item) {
	if jsonOut {
		return
	}
	clearProgress()
	say("%s    %s", color("1", title), freeSummary())
	group := ""
	for _, it := range items {
		if it.Group != group {
			group = it.Group
			say("\n%s", color("1;36", group))
		}
		mark, note := color("32", "✓"), ""
		if !it.Selected {
			mark = color("2", "○")
		}
		if it.Risk == "info" {
			mark = color("33", "!")
		} else if it.Risk != "safe" {
			note = color("33", " ("+it.Risk+")")
		}
		if it.Note != "" {
			note += color("2", " · "+it.Note)
		}
		size := human(it.Bytes)
		if it.Bytes == 0 {
			size = "-"
		}
		say("  %s %-42s %10s%s", mark, it.Label, size, note)
	}
}

// human formats bytes in decimal units, as Mole and most desktop UIs do.
func human(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func home() string { h, _ := os.UserHomeDir(); return h }

// expand turns "~/x" into an absolute path.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), p[1:])
	}
	return p
}

// globs expands ~ and shell globs, returning existing paths.
func globs(patterns ...string) []string {
	var out []string
	for _, p := range patterns {
		m, _ := filepath.Glob(expand(p))
		out = append(out, m...)
	}
	return out
}

// children lists the entries directly inside dir.
func children(dir string) []string {
	ents, err := os.ReadDir(expand(dir))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, filepath.Join(expand(dir), e.Name()))
	}
	return out
}

// diskUsage returns allocated bytes under path, like du.
func diskUsage(path string) int64 {
	s := newScan(path)
	s.walkParallel(path)
	return s.total()
}

// freeable returns the bytes deleting path would actually free.
func freeable(path string) int64 {
	s := newScan(path)
	s.freeable = true
	s.walkParallel(path)
	return s.total()
}

// entries sizes each path concurrently and drops empty ones.
func entries(paths []string) []Entry {
	out := make([]Entry, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			out[i] = Entry{Path: p, Bytes: freeable(p)}
			<-sem
		}()
	}
	wg.Wait()
	kept := out[:0]
	for _, e := range out {
		if e.Bytes > 0 {
			kept = append(kept, e)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Bytes > kept[j].Bytes })
	return kept
}

func sum(es []Entry) (t int64) {
	for _, e := range es {
		t += e.Bytes
	}
	return
}

// forceRemove deletes a validated path; on permission errors it makes the
// user's own read-only dirs writable (Go module cache, uv) and retries.
func forceRemove(path string) error {
	err := safe.Remove(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	uid := uint32(os.Getuid())
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if info, e := d.Info(); e == nil {
				if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid == uid {
					os.Chmod(p, info.Mode().Perm()|0o700)
				}
			}
		}
		return nil
	})
	return safe.Remove(path)
}

// freeSummary reports free space on / and, when it is a separate disk, on the home folder.
func freeSummary() string {
	out := "Free: / " + human(freeBytes("/"))
	var a, b syscall.Stat_t
	if syscall.Stat("/", &a) == nil && syscall.Stat(home(), &b) == nil && a.Dev != b.Dev {
		out += " · home " + human(freeBytes(home()))
	}
	return out
}

func freeBytes(path string) int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0
	}
	return int64(st.Bavail) * st.Bsize
}

// running reports whether a process whose comm matches one of names is alive.
func running(names ...string) bool {
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + p.Name() + "/comm")
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(b))
		for _, n := range names {
			if comm == n {
				return true
			}
		}
	}
	return false
}

// lockedBy reports whether a Chromium/Electron/Firefox-style lock symlink
// ("host-PID" or "IP:+PID") points to a live process.
func lockedBy(lock string) bool {
	target, err := os.Readlink(lock)
	if err != nil {
		return false
	}
	i := strings.LastIndexAny(target, "-+")
	if i < 0 {
		return false
	}
	_, err = os.Stat("/proc/" + target[i+1:])
	return err == nil
}

// cmdOut runs a command and returns trimmed stdout ("" on error).
func cmdOut(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func have(name string) bool { _, err := exec.LookPath(name); return err == nil }

// whitelist holds paths the user never wants touched (~/.config/preen/whitelist).
// An unreadable whitelist stops the command rather than running without it.
func loadWhitelist() ([]string, error) {
	f := filepath.Join(configDir(), "whitelist")
	b, err := os.ReadFile(f)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read whitelist %s: %w", f, err)
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, filepath.Clean(expand(l)))
		}
	}
	return out, nil
}

func whitelisted(wl []string, p string) bool {
	for _, w := range wl {
		if p == w || strings.HasPrefix(p, w+"/") || strings.HasPrefix(w, p+"/") {
			return true
		}
		if ok, _ := filepath.Match(w, p); ok {
			return true
		}
	}
	return false
}

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "preen")
	}
	return filepath.Join(home(), ".config", "preen")
}

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "preen")
	}
	return filepath.Join(home(), ".local", "state", "preen")
}

// ---- operations log (JSONL) ----

type oplogEntry struct {
	Time   time.Time `json:"time"`
	Run    string    `json:"run"`
	Cmd    string    `json:"cmd,omitempty"`
	Action string    `json:"action"`
	ID     string    `json:"id,omitempty"`
	Path   string    `json:"path,omitempty"`
	Bytes  int64     `json:"bytes,omitempty"`
	Err    string    `json:"error,omitempty"`
}

type opRun struct{ id, cmd string }

func newRun(cmd string) *opRun {
	r := &opRun{id: strconv.FormatInt(time.Now().UnixNano(), 36), cmd: cmd}
	r.log(oplogEntry{Action: "start"})
	return r
}

func (r *opRun) log(e oplogEntry) {
	if os.Getenv("PREEN_NO_OPLOG") != "" {
		return
	}
	e.Time, e.Run, e.Cmd = time.Now(), r.id, r.cmd
	os.MkdirAll(stateDir(), 0o700)
	f, err := os.OpenFile(filepath.Join(stateDir(), "operations.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(e)
}
