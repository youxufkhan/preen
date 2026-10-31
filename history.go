package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func cmdHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	fs.BoolVar(&jsonOut, "json", false, "print NDJSON events")
	limit := fs.Int("n", 50, "number of runs to show")
	fs.Parse(args)

	type runSum struct {
		e             oplogEntry
		freed         int64
		items, failed int
		ended         bool
	}
	runs := map[string]*runSum{}
	f, err := os.Open(filepath.Join(stateDir(), "operations.log"))
	if err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var e oplogEntry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			r := runs[e.Run]
			if r == nil {
				r = &runSum{e: e}
				runs[e.Run] = r
			}
			switch {
			case e.Action == "end":
				r.freed, r.ended = e.Bytes, true
			case e.Action == "item" || e.Action == "trash" && e.Cmd == "trash":
				if e.Err != "" {
					r.failed++
				} else {
					r.items++
				}
			}
		}
		f.Close()
	}
	list := make([]*runSum, 0, len(runs))
	for _, r := range runs {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].e.Time.After(list[j].e.Time) })
	if len(list) > *limit {
		list = list[:*limit]
	}
	say("%s", color("1", "Preen history"))
	for _, r := range list {
		note := fmt.Sprintf("%d items", r.items)
		if r.failed > 0 {
			note += fmt.Sprintf(", %d failed", r.failed)
		}
		if !r.ended {
			note += ", interrupted"
		}
		emit(Event{Type: "item", Item: &Item{ID: r.e.Run, Group: r.e.Cmd, Label: r.e.Time.Local().Format("2006-01-02 15:04"), Bytes: r.freed, Note: note, Risk: "safe"}})
		say("  %s  %-10s %10s  %s", r.e.Time.Local().Format("2006-01-02 15:04"), r.e.Cmd, human(r.freed), color("2", note))
	}
	if len(list) == 0 {
		say("  Nothing yet.")
	}
	emit(Event{Type: "done", Count: len(list)})
	return nil
}
