package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Status struct {
	Host     string    `json:"host"`
	OS       string    `json:"os"`
	Kernel   string    `json:"kernel"`
	Uptime   int64     `json:"uptime_seconds"`
	Health   int       `json:"health_score"`
	CPU      CPUStat   `json:"cpu"`
	Memory   MemStat   `json:"memory"`
	Pressure Pressure  `json:"pressure"`
	Disks    []DiskSt  `json:"disks"`
	DiskIO   Rate      `json:"disk_io"`
	Network  Rate      `json:"network"`
	Battery  *Battery  `json:"battery,omitempty"`
	Temps    []Temp    `json:"temps"`
	Procs    []ProcSt  `json:"top_processes"`
	Time     time.Time `json:"time"`
}

type CPUStat struct {
	Model string     `json:"model"`
	Usage float64    `json:"usage"`
	Cores []float64  `json:"cores"`
	Load  [3]float64 `json:"load"`
}
type MemStat struct {
	Total     int64 `json:"total"`
	Used      int64 `json:"used"`
	Available int64 `json:"available"`
	SwapTotal int64 `json:"swap_total"`
	SwapUsed  int64 `json:"swap_used"`
}
type Pressure struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	IO     float64 `json:"io"`
}
type DiskSt struct {
	Mount string `json:"mount"`
	FS    string `json:"fs"`
	Total int64  `json:"total"`
	Used  int64  `json:"used"`
	Free  int64  `json:"free"`
}
type Rate struct {
	Read  float64 `json:"read_bps,omitempty"`
	Write float64 `json:"write_bps,omitempty"`
	Down  float64 `json:"down_bps,omitempty"`
	Up    float64 `json:"up_bps,omitempty"`
	Iface string  `json:"iface,omitempty"`
}
type Battery struct {
	Percent  int    `json:"percent"`
	Status   string `json:"status"`
	HealthPc int    `json:"health_percent,omitempty"`
	Cycles   int    `json:"cycles,omitempty"`
}
type Temp struct {
	Label   string  `json:"label"`
	Celsius float64 `json:"celsius"`
}
type ProcSt struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	CPU  float64 `json:"cpu"`
	RSS  int64   `json:"rss"`
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.BoolVar(&jsonOut, "json", false, "print JSON (NDJSON with --watch)")
	watch := fs.Bool("watch", false, "keep updating")
	interval := fs.Duration("interval", 2*time.Second, "update interval with --watch")
	fs.Parse(args)
	prev := sample()
	time.Sleep(500 * time.Millisecond)
	for {
		cur := sample()
		st := buildStatus(prev, cur)
		emit(Event{Type: "status", Data: st})
		if !jsonOut {
			if *watch {
				fmt.Print("\033[H\033[2J")
			}
			printStatus(st)
		}
		if !*watch {
			return nil
		}
		prev = cur
		time.Sleep(*interval)
	}
}

// raw counters; rates come from the difference between two samples.
type raw struct {
	at    time.Time
	cpu   [][2]uint64 // [busy, total] for total then each core
	procs map[int]uint64
	diskR uint64
	diskW uint64
	netRx uint64
	netTx uint64
	iface string
}

func sample() raw {
	r := raw{at: time.Now(), procs: map[int]uint64{}}
	if f, err := os.Open("/proc/stat"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fl := strings.Fields(sc.Text())
			if len(fl) < 8 || !strings.HasPrefix(fl[0], "cpu") {
				continue
			}
			var total, idle uint64
			for i, v := range fl[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				total += n
				if i == 3 || i == 4 { // idle, iowait
					idle += n
				}
			}
			r.cpu = append(r.cpu, [2]uint64{total - idle, total})
		}
		f.Close()
	}
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		if b, err := os.ReadFile("/proc/" + p.Name() + "/stat"); err == nil {
			s := string(b)
			fl := strings.Fields(s[strings.LastIndex(s, ")")+1:])
			if len(fl) > 12 {
				u, _ := strconv.ParseUint(fl[11], 10, 64)
				k, _ := strconv.ParseUint(fl[12], 10, 64)
				r.procs[pid] = u + k
			}
		}
	}
	for _, l := range lines("/proc/diskstats") {
		fl := strings.Fields(l)
		if len(fl) < 10 || !isWholeDisk(fl[2]) {
			continue
		}
		rd, _ := strconv.ParseUint(fl[5], 10, 64)
		wr, _ := strconv.ParseUint(fl[9], 10, 64)
		r.diskR += rd * 512
		r.diskW += wr * 512
	}
	r.iface = defaultIface()
	for _, l := range lines("/proc/net/dev") {
		name, rest, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok || name != r.iface {
			continue
		}
		fl := strings.Fields(rest)
		if len(fl) >= 9 {
			r.netRx, _ = strconv.ParseUint(fl[0], 10, 64)
			r.netTx, _ = strconv.ParseUint(fl[8], 10, 64)
		}
	}
	return r
}

func isWholeDisk(name string) bool {
	_, err := os.Stat("/sys/block/" + name + "/device")
	return err == nil
}

func defaultIface() string {
	for _, l := range lines("/proc/net/route")[1:] {
		if fl := strings.Fields(l); len(fl) > 2 && fl[1] == "00000000" {
			return fl[0]
		}
	}
	return ""
}

func lines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{""}
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func buildStatus(a, b raw) Status {
	st := Status{Time: b.at, Temps: []Temp{}, Disks: []DiskSt{}}
	st.Host, _ = os.Hostname()
	st.OS = osName()
	var u syscall.Utsname
	if syscall.Uname(&u) == nil {
		st.Kernel = cstr(u.Release[:])
	}
	if f := strings.Fields(lines("/proc/uptime")[0]); len(f) > 0 {
		up, _ := strconv.ParseFloat(f[0], 64)
		st.Uptime = int64(up)
	}
	secs := b.at.Sub(a.at).Seconds()
	pct := func(x, y [2]uint64) float64 {
		if y[1] <= x[1] {
			return 0
		}
		return round1(float64(y[0]-x[0]) / float64(y[1]-x[1]) * 100)
	}
	if len(a.cpu) == len(b.cpu) && len(b.cpu) > 0 {
		st.CPU.Usage = pct(a.cpu[0], b.cpu[0])
		for i := 1; i < len(b.cpu); i++ {
			st.CPU.Cores = append(st.CPU.Cores, pct(a.cpu[i], b.cpu[i]))
		}
	}
	for _, l := range lines("/proc/cpuinfo") {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "model name" {
			st.CPU.Model = strings.TrimSpace(v)
			break
		}
	}
	if f := strings.Fields(lines("/proc/loadavg")[0]); len(f) >= 3 {
		for i := range 3 {
			st.CPU.Load[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}
	mem := map[string]int64{}
	for _, l := range lines("/proc/meminfo") {
		if k, v, ok := strings.Cut(l, ":"); ok {
			n, _ := strconv.ParseInt(strings.Fields(v)[0], 10, 64)
			mem[k] = n * 1024
		}
	}
	st.Memory = MemStat{Total: mem["MemTotal"], Available: mem["MemAvailable"], Used: mem["MemTotal"] - mem["MemAvailable"],
		SwapTotal: mem["SwapTotal"], SwapUsed: mem["SwapTotal"] - mem["SwapFree"]}
	st.Pressure = Pressure{CPU: psi("cpu"), Memory: psi("memory"), IO: psi("io")}
	st.Disks = disks()
	if secs > 0 {
		st.DiskIO = Rate{Read: float64(b.diskR-a.diskR) / secs, Write: float64(b.diskW-a.diskW) / secs}
		if a.iface == b.iface {
			st.Network = Rate{Down: float64(b.netRx-a.netRx) / secs, Up: float64(b.netTx-a.netTx) / secs, Iface: b.iface}
		}
	}
	st.Battery = battery()
	st.Temps = temps()
	st.Procs = topProcs(a, b, secs)
	st.Health = health(st)
	return st
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func cstr(b []int8) string {
	var s []byte
	for _, c := range b {
		if c == 0 {
			break
		}
		s = append(s, byte(c))
	}
	return string(s)
}

func osName() string {
	for _, l := range lines("/etc/os-release") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "Linux"
}

var realFS = map[string]bool{"ext4": true, "ext3": true, "btrfs": true, "xfs": true, "f2fs": true, "vfat": true, "exfat": true, "ntfs": true, "ntfs3": true, "fuseblk": true, "zfs": true}

func disks() []DiskSt {
	var out []DiskSt
	seen := map[string]bool{}
	for _, l := range lines("/proc/mounts") {
		f := strings.Fields(l)
		if len(f) < 3 || !realFS[f[2]] || seen[f[0]] || strings.HasPrefix(f[1], "/snap") || f[1] == "/boot/efi" {
			continue
		}
		seen[f[0]] = true
		mount := strings.ReplaceAll(f[1], `\040`, " ")
		total, free := diskSpace(mount)
		if total == 0 {
			continue
		}
		out = append(out, DiskSt{Mount: mount, FS: f[2], Total: total, Free: free, Used: total - free})
	}
	return out
}

func statfs(path string) (syscall.Statfs_t, error) {
	var st syscall.Statfs_t
	return st, syscall.Statfs(path, &st)
}

func readInt(path string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(lines(path)[0]), 10, 64)
	return n
}

func battery() *Battery {
	for _, d := range globs("/sys/class/power_supply/BAT*") {
		b := &Battery{Percent: int(readInt(d + "/capacity")), Status: lines(d + "/status")[0], Cycles: int(readInt(d + "/cycle_count"))}
		full, design := readInt(d+"/energy_full"), readInt(d+"/energy_full_design")
		if design == 0 {
			full, design = readInt(d+"/charge_full"), readInt(d+"/charge_full_design")
		}
		if design > 0 {
			b.HealthPc = int(full * 100 / design)
		}
		return b
	}
	return nil
}

// temps reads the most useful sensor of each hwmon chip (CPU package, GPU edge, NVMe composite).
func temps() []Temp {
	out := []Temp{}
	for _, d := range globs("/sys/class/hwmon/hwmon*") {
		chip := lines(d + "/name")[0]
		ins := globs(d + "/temp*_input")
		if len(ins) == 0 {
			continue
		}
		best, bestLabel := ins[0], ""
		for _, in := range ins {
			label := lines(strings.TrimSuffix(in, "_input") + "_label")[0]
			if strings.HasPrefix(label, "Package") || label == "Tctl" || label == "edge" || label == "Composite" {
				best, bestLabel = in, label
				break
			}
		}
		c := float64(readInt(best)) / 1000
		if c <= 0 || c > 150 {
			continue
		}
		name := map[string]string{"coretemp": "CPU", "k10temp": "CPU", "zenpower": "CPU", "amdgpu": "GPU", "nvme": "SSD",
			"acpitz": "Board", "nouveau": "GPU", "thinkpad": "Board", "dell_smm": "Board"}[chip]
		if strings.HasPrefix(chip, "iwlwifi") {
			name = "Wi-Fi"
		}
		if name == "" {
			name = chip
		}
		if bestLabel != "" && name == chip {
			name += " " + bestLabel
		}
		out = append(out, Temp{Label: name, Celsius: round1(c)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Celsius > out[j].Celsius })
	return out
}

func topProcs(a, b raw, secs float64) []ProcSt {
	const hz = 100 // USER_HZ on every mainstream Linux build
	page := int64(os.Getpagesize())
	var out []ProcSt
	for pid, t := range b.procs {
		prev, ok := a.procs[pid]
		if !ok || t < prev || secs == 0 {
			continue
		}
		cpu := float64(t-prev) / hz / secs * 100
		if cpu < 0.5 {
			continue
		}
		p := ProcSt{PID: pid, CPU: round1(cpu)}
		p.Name = strings.TrimSpace(lines("/proc/" + strconv.Itoa(pid) + "/comm")[0])
		if p.Name == "preen" {
			continue // our own scans are not what the user is looking for
		}
		if f := strings.Fields(lines("/proc/" + strconv.Itoa(pid) + "/statm")[0]); len(f) > 1 {
			rss, _ := strconv.ParseInt(f[1], 10, 64)
			p.RSS = rss * page
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CPU > out[j].CPU })
	if len(out) > 6 {
		out = out[:6]
	}
	if out == nil {
		out = []ProcSt{}
	}
	return out
}

// health combines load, memory, disk space, pressure, heat, battery and uptime into 0–100.
func health(s Status) int {
	h := 100
	pen := func(cond bool, n int) {
		if cond {
			h -= n
		}
	}
	pen(s.CPU.Usage > 85, 15)
	if s.Memory.Total > 0 {
		used := float64(s.Memory.Used) / float64(s.Memory.Total)
		pen(used > 0.9, 20)
		pen(used > 0.8 && used <= 0.9, 10)
	}
	for _, d := range s.Disks {
		if d.Mount == "/" && d.Total > 0 {
			free := float64(d.Free) / float64(d.Total)
			pen(free < 0.1, 20)
			pen(free >= 0.1 && free < 0.2, 10)
		}
	}
	pen(s.Pressure.Memory > 10, 10)
	if len(s.Temps) > 0 {
		pen(s.Temps[0].Celsius > 90, 15)
		pen(s.Temps[0].Celsius > 80 && s.Temps[0].Celsius <= 90, 5)
	}
	pen(s.Battery != nil && s.Battery.HealthPc > 0 && s.Battery.HealthPc < 60, 5)
	pen(s.Uptime > 30*86400, 5)
	return max(h, 0)
}

func bar(pct float64, width int) string {
	n := int(pct / 100 * float64(width))
	n = min(max(n, 0), width)
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

func printStatus(s Status) {
	hc := "32"
	if s.Health < 75 {
		hc = "33"
	}
	if s.Health < 50 {
		hc = "31"
	}
	say("%s  Health %s  %s · %s · up %s", color("1", "Preen status"), color(hc, fmt.Sprintf("● %d", s.Health)), s.Host, s.OS, uptime(s.Uptime))
	say("\n%s  %s", color("1;36", "CPU"), color("2", s.CPU.Model))
	say("  Total  %s %5.1f%%   Load %.2f / %.2f / %.2f (%d cores)", bar(s.CPU.Usage, 20), s.CPU.Usage, s.CPU.Load[0], s.CPU.Load[1], s.CPU.Load[2], len(s.CPU.Cores))
	m := s.Memory
	say("\n%s", color("1;36", "Memory"))
	say("  Used   %s %5.1f%%   %s of %s · swap %s", bar(pctOf(m.Used, m.Total), 20), pctOf(m.Used, m.Total), human(m.Used), human(m.Total), human(m.SwapUsed))
	say("\n%s", color("1;36", "Disks"))
	for _, d := range s.Disks {
		say("  %-6s %s %5.1f%%   %s free of %s", filepath.Base(d.Mount), bar(pctOf(d.Used, d.Total), 20), pctOf(d.Used, d.Total), human(d.Free), human(d.Total))
	}
	say("  I/O    read %s/s · write %s/s", human(int64(s.DiskIO.Read)), human(int64(s.DiskIO.Write)))
	say("\n%s  %s", color("1;36", "Network"), color("2", s.Network.Iface))
	say("  Down %s/s · Up %s/s", human(int64(s.Network.Down)), human(int64(s.Network.Up)))
	if b := s.Battery; b != nil {
		say("\n%s", color("1;36", "Battery"))
		line := fmt.Sprintf("  %d%% · %s · health %d%%", b.Percent, b.Status, b.HealthPc)
		if b.Cycles > 0 {
			line += fmt.Sprintf(" · %d cycles", b.Cycles)
		}
		say("%s", line)
	}
	if len(s.Temps) > 0 {
		var t []string
		for _, x := range s.Temps {
			t = append(t, fmt.Sprintf("%s %.0f°C", x.Label, x.Celsius))
		}
		say("\n%s  %s", color("1;36", "Sensors"), strings.Join(t, " · "))
	}
	say("\n%s", color("1;36", "Top processes"))
	for _, p := range s.Procs {
		say("  %-20s %5.1f%%  %s", p.Name, p.CPU, human(p.RSS))
	}
}

func uptime(sec int64) string {
	d, h, m := sec/86400, sec%86400/3600, sec%3600/60
	if d > 0 {
		return fmt.Sprintf("%dd %dh", d, h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func pctOf(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}
