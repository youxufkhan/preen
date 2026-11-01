package main

import (
	"container/heap"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"preen/internal/safe"
)

type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	IsDir bool   `json:"is_dir"`
	Files int64  `json:"files,omitempty"`
}

type Analysis struct {
	Path       string     `json:"path"`
	Overview   bool       `json:"overview"`
	Entries    []DirEntry `json:"entries"`
	LargeFiles []DirEntry `json:"large_files"`
	Total      int64      `json:"total"`
	Files      int64      `json:"files"`
	Partial    bool       `json:"partial"` // some folders could not be read
	DiskTotal  int64      `json:"disk_total"`
	DiskFree   int64      `json:"disk_free"`
}

// Locations shown when no path is given. They overlap on purpose (Home contains .cache).
var overview = []struct{ label, path string }{
	{"Home", "~"}, {"App caches", "~/.cache"}, {"App settings", "~/.config"}, {"App data", "~/.local/share"},
	{"Snap app data", "~/snap"}, {"Downloads", "~/Downloads"}, {"Trash", "~/.local/share/Trash"},
	{"Installed snaps", "/var/lib/snapd/snaps"}, {"Package cache", "/var/cache"}, {"System logs", "/var/log"},
	{"Programs", "/usr"}, {"Optional software", "/opt"},
}

func cmdAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	fs.BoolVar(&jsonOut, "json", false, "print JSON")
	fs.Parse(args)
	var a Analysis
	if fs.NArg() == 0 {
		a = analyzeOverview()
	} else {
		p, err := filepath.Abs(expand(fs.Arg(0)))
		if err != nil {
			return err
		}
		a = analyzeDir(p)
	}
	clearProgress()
	emit(Event{Type: "analysis", Data: a})
	if jsonOut {
		return nil
	}
	title := "Disk usage: " + tildePath(a.Path)
	if a.Overview {
		title = "Disk overview"
	}
	say("%s    %s free of %s", color("1", title), human(a.DiskFree), human(a.DiskTotal))
	var max int64 = 1
	for _, e := range a.Entries {
		max = maxI(max, e.Bytes)
	}
	for i, e := range a.Entries {
		if i == 30 {
			say(color("2", "  … %d more"), len(a.Entries)-30)
			break
		}
		bar := strings.Repeat("█", int(e.Bytes*24/max)) + strings.Repeat("░", 24-int(e.Bytes*24/max))
		name := e.Name
		if e.IsDir && !a.Overview {
			name += "/"
		}
		say("  %s %10s  %s", color("36", bar), human(e.Bytes), name)
	}
	if len(a.LargeFiles) > 0 {
		say("\n%s", color("1", "Largest files"))
		for _, f := range a.LargeFiles {
			say("  %10s  %s", human(f.Bytes), tildePath(f.Path))
		}
	}
	if a.Partial {
		say(color("33", "\nSome folders could not be read; sizes are a minimum."))
	}
	return nil
}

// Files and bytes seen by every scan in this process, for progress reports.
var liveFiles, liveBytes atomic.Int64

// tracker reports which folders are being measured, every 300 ms, until stop.
type tracker struct {
	mu      sync.Mutex
	running []string
	done    int
	total   int
	quit    chan struct{}
}

func newTracker(total int) *tracker {
	t := &tracker{total: total, quit: make(chan struct{})}
	go func() {
		tick := time.NewTicker(300 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-t.quit:
				return
			case <-tick.C:
				t.report()
			}
		}
	}()
	return t
}

func (t *tracker) start(name string) {
	t.mu.Lock()
	t.running = append(t.running, name)
	t.mu.Unlock()
}

func (t *tracker) finish(name string) {
	t.mu.Lock()
	t.running = slices.DeleteFunc(t.running, func(n string) bool { return n == name })
	t.done++
	t.mu.Unlock()
}

func (t *tracker) report() {
	t.mu.Lock()
	var msg string
	switch n := len(t.running); {
	case n == 0:
		msg = "Finishing"
	case n == 1:
		msg = "Measuring " + t.running[0]
	case n == 2:
		msg = "Measuring " + t.running[0] + " and " + t.running[1]
	default:
		msg = fmt.Sprintf("Measuring %s, %s and %d more", t.running[0], t.running[1], n-2)
	}
	done, total := t.done, t.total
	t.mu.Unlock()
	progressN(msg, done, total, liveBytes.Load(), liveFiles.Load())
}

func (t *tracker) stop() { close(t.quit) }

func maxI(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func analyzeOverview() Analysis {
	a := Analysis{Path: home(), Overview: true}
	a.DiskTotal, a.DiskFree = diskSpace(home())
	res := make([]DirEntry, len(overview))
	var wg sync.WaitGroup
	var partial atomic.Bool
	tr := newTracker(len(overview))
	defer tr.stop()
	for i, o := range overview {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.start(o.label)
			p := expand(o.path)
			s := newScan(p)
			s.walkParallel(p)
			res[i] = DirEntry{Name: o.label, Path: p, Bytes: s.bytes.Load(), IsDir: true, Files: s.files.Load()}
			if res[i].Bytes > 0 {
				emit(Event{Type: "entry", Data: res[i]}) // the GUI shows each location as soon as it is measured
			}
			if s.partial.Load() {
				partial.Store(true)
			}
			tr.finish(o.label)
		}()
	}
	wg.Wait()
	for _, e := range res {
		if e.Bytes > 0 {
			a.Entries = append(a.Entries, e)
		}
	}
	sort.SliceStable(a.Entries, func(i, j int) bool { return a.Entries[i].Bytes > a.Entries[j].Bytes })
	a.Partial = partial.Load()
	a.LargeFiles = []DirEntry{}
	return a
}

// analyzeDir sizes each direct child of root in parallel and keeps the largest files.
func analyzeDir(root string) Analysis {
	a := Analysis{Path: root}
	a.DiskTotal, a.DiskFree = diskSpace(root)
	s := newScan(root)
	ents, err := os.ReadDir(root)
	if err != nil {
		a.Partial = true
	}
	res := make([]DirEntry, len(ents))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	tr := newTracker(len(ents))
	defer tr.stop()
	for i, e := range ents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tr.start(e.Name())
			p := filepath.Join(root, e.Name())
			sub := s.child()
			sub.walkParallel(p)
			s.merge(sub)
			res[i] = DirEntry{Name: e.Name(), Path: p, Bytes: sub.bytes.Load(), IsDir: e.IsDir(), Files: sub.files.Load()}
			if e.IsDir() {
				emit(Event{Type: "entry", Data: res[i]}) // folders appear as they finish; files come with the result
			}
			tr.finish(e.Name())
		}()
	}
	wg.Wait()
	a.Entries = res
	sort.SliceStable(a.Entries, func(i, j int) bool { return a.Entries[i].Bytes > a.Entries[j].Bytes })
	a.Total, a.Files, a.Partial = s.bytes.Load(), s.files.Load(), a.Partial || s.partial.Load()
	a.LargeFiles = s.largest()
	return a
}

// scan accumulates sizes and stays on one filesystem. In du mode a hard-linked
// file counts once; in freeable mode it counts only when every link is inside
// the scan, so a cache whose files are linked into projects (uv, pnpm, bun)
// is not reported as space it would free.
type scan struct {
	dev      uint64
	freeable bool
	bytes    atomic.Int64
	files    atomic.Int64
	partial  atomic.Bool
	sh       *shared
}

type shared struct {
	mu    sync.Mutex
	links map[uint64]*link
	big   fileHeap
}

type link struct {
	seen, nlink uint64
	size        int64
}

const largeFile = 50e6

var walkSem = make(chan struct{}, 32)

func newScan(root string) *scan {
	s := &scan{sh: &shared{links: map[uint64]*link{}}}
	var st syscall.Stat_t
	if syscall.Stat(root, &st) == nil {
		s.dev = st.Dev
	}
	return s
}

// child shares the link table and large-file list but counts its own bytes.
func (s *scan) child() *scan { return &scan{dev: s.dev, freeable: s.freeable, sh: s.sh} }

func (s *scan) merge(c *scan) {
	s.bytes.Add(c.bytes.Load())
	s.files.Add(c.files.Load())
	if c.partial.Load() {
		s.partial.Store(true)
	}
}

// total returns the scanned bytes, adding hard-linked files fully inside the scan in freeable mode.
func (s *scan) total() int64 {
	t := s.bytes.Load()
	if s.freeable {
		for _, l := range s.sh.links {
			if l.seen >= l.nlink {
				t += l.size
			}
		}
	}
	return t
}

var skipRoots = map[string]bool{"/proc": true, "/sys": true, "/dev": true, "/run": true, "/snap": true}

// walkParallel walks each top-level subfolder of root in its own goroutine.
func (s *scan) walkParallel(root string) {
	ents, err := os.ReadDir(root)
	if err != nil {
		s.walk(root) // a file, or unreadable (walk records partial)
		return
	}
	if info, err := os.Lstat(root); err == nil {
		s.bytes.Add(info.Sys().(*syscall.Stat_t).Blocks * 512)
	}
	var wg sync.WaitGroup
	for _, e := range ents {
		p := filepath.Join(root, e.Name())
		if !e.IsDir() {
			s.walk(p)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			walkSem <- struct{}{}
			defer func() { <-walkSem }()
			s.walk(p)
		}()
	}
	wg.Wait()
}

func (s *scan) walk(root string) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.partial.Store(true)
			return nil
		}
		if skipRoots[p] {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		if d.IsDir() && s.dev != 0 && st.Dev != s.dev {
			return fs.SkipDir // another filesystem (snap squashfs, USB disk, network mount)
		}
		size := st.Blocks * 512
		if !d.IsDir() {
			s.files.Add(1)
			liveFiles.Add(1)
		}
		if !d.IsDir() && st.Nlink > 1 {
			s.sh.mu.Lock()
			l := s.sh.links[st.Ino]
			first := l == nil
			if first {
				l = &link{nlink: uint64(st.Nlink), size: size}
				s.sh.links[st.Ino] = l
			}
			l.seen++
			s.sh.mu.Unlock()
			if s.freeable || !first {
				return nil
			}
		}
		s.bytes.Add(size)
		liveBytes.Add(size)
		if !d.IsDir() && size >= largeFile {
			s.sh.mu.Lock()
			heap.Push(&s.sh.big, DirEntry{Name: d.Name(), Path: p, Bytes: size})
			if s.sh.big.Len() > 20 {
				heap.Pop(&s.sh.big)
			}
			s.sh.mu.Unlock()
		}
		return nil
	})
}

func (s *scan) largest() []DirEntry {
	out := append([]DirEntry{}, s.sh.big...)
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// fileHeap is a min-heap by size, so the smallest of the top 20 is dropped first.
type fileHeap []DirEntry

func (h fileHeap) Len() int           { return len(h) }
func (h fileHeap) Less(i, j int) bool { return h[i].Bytes < h[j].Bytes }
func (h fileHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *fileHeap) Push(x any)        { *h = append(*h, x.(DirEntry)) }
func (h *fileHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

func cmdTrash(args []string) error {
	fs := flag.NewFlagSet("trash", flag.ExitOnError)
	fs.BoolVar(&jsonOut, "json", false, "print NDJSON events")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: preen trash <path>...")
	}
	r := newRun("trash")
	var freed int64
	var n int
	for _, p := range fs.Args() {
		p, _ = filepath.Abs(expand(p))
		b := diskUsage(p)
		err := safe.Trash(p)
		r.log(oplogEntry{Action: "trash", Path: p, Bytes: b, Err: errString(err)})
		if err != nil {
			emit(Event{Type: "result", ID: p, Status: "failed", Message: err.Error()})
			say("  %s %s: %v", color("31", "✗"), tildePath(p), err)
			continue
		}
		freed += b
		n++
		emit(Event{Type: "result", ID: p, Status: "applied", Bytes: b})
		say("  %s Moved to Trash: %s", color("32", "✓"), tildePath(p))
	}
	r.log(oplogEntry{Action: "end", Bytes: freed})
	emit(Event{Type: "done", Bytes: freed, Count: n})
	return nil
}
