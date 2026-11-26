package main

// Whole-host sampling. The per-container numbers say what each service needs; these say whether the host
// they share is big enough, which is what choosing a VPS comes down to:
//
//   - CPU busy, steal and iowait from /proc/stat. Steal is time the hypervisor gave to someone else while
//     this machine had work: on a shared-vCPU VPS it is how an oversold host shows up.
//   - Pressure stall information (PSI, /proc/pressure/*): the share of wall time some (or all) tasks
//     waited for CPU, memory or disk. Unlike utilisation it says when work was actually held back.
//   - MemAvailable (what could still be handed out without swapping) and swap in use.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// cpuTimes are the cumulative jiffies of the "cpu" line of /proc/stat.
type cpuTimes struct {
	user, nice, system, idle, iowait, irq, softirq, steal int64
}

func (c cpuTimes) total() int64 {
	return c.user + c.nice + c.system + c.idle + c.iowait + c.irq + c.softirq + c.steal
}

// psiTotals are the cumulative microseconds a pressure file reports.
type psiTotals struct {
	some, full int64
}

// nodeReading is the host's counters at one moment. psi is empty when the kernel has pressure information
// off (CONFIG_PSI, or psi=0 on the command line).
type nodeReading struct {
	at              time.Time
	cpu             cpuTimes
	psi             map[string]psiTotals
	memAvailableKiB int64
	swapUsedKiB     int64
}

var pressureResources = []string{"cpu", "memory", "io"}

// nodeSample is one interval: percentages of the interval's wall time (or of all CPU time for the cpu
// ones), and memory in MiB. Pressure values are NaN-free: a host without PSI leaves hasPSI false.
type nodeSample struct {
	busyPct, stealPct, iowaitPct     float64
	cpuSome                          float64
	memSome, memFull, ioSome, ioFull float64
	hasPSI                           bool
	memAvailableMiB, swapUsedMiB     float64
}

func readNode() (nodeReading, error) {
	r := nodeReading{at: time.Now(), psi: map[string]psiTotals{}}

	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nodeReading{}, err
	}
	if r.cpu, err = parseCPUStat(string(stat)); err != nil {
		return nodeReading{}, err
	}

	info, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nodeReading{}, err
	}
	if r.memAvailableKiB, r.swapUsedKiB, err = parseMeminfo(string(info)); err != nil {
		return nodeReading{}, err
	}

	for _, resource := range pressureResources {
		raw, err := os.ReadFile("/proc/pressure/" + resource)
		if err != nil {
			// PSI is off or missing: say so in the report instead of failing the run
			r.psi = map[string]psiTotals{}
			break
		}
		totals, err := parsePressure(string(raw))
		if err != nil {
			return nodeReading{}, fmt.Errorf("/proc/pressure/%s: %w", resource, err)
		}
		r.psi[resource] = totals
	}

	return r, nil
}

// parseCPUStat reads the aggregate "cpu" line: user nice system idle iowait irq softirq steal ...
func parseCPUStat(text string) (cpuTimes, error) {
	for line := range strings.SplitSeq(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 9 {
			return cpuTimes{}, fmt.Errorf("cpu line has %d fields, want at least 9", len(fields))
		}

		var v [8]int64
		for i := range v {
			n, err := strconv.ParseInt(fields[i+1], 10, 64)
			if err != nil {
				return cpuTimes{}, fmt.Errorf("cpu line: %w", err)
			}
			v[i] = n
		}

		return cpuTimes{user: v[0], nice: v[1], system: v[2], idle: v[3], iowait: v[4], irq: v[5], softirq: v[6], steal: v[7]}, nil
	}

	return cpuTimes{}, errors.New("no cpu line")
}

// parsePressure reads "some avg10=0.00 avg60=0.00 avg300=0.00 total=123" (and the "full" line, which the cpu
// file may lack) and returns the cumulative totals in microseconds.
func parsePressure(text string) (psiTotals, error) {
	var totals psiTotals
	found := false

	for line := range strings.SplitSeq(text, "\n") {
		kind, rest, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || (kind != "some" && kind != "full") {
			continue
		}

		for field := range strings.FieldsSeq(rest) {
			key, value, ok := strings.Cut(field, "=")
			if !ok || key != "total" {
				continue
			}

			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return psiTotals{}, fmt.Errorf("%s total: %w", kind, err)
			}
			if kind == "some" {
				totals.some = n
			} else {
				totals.full = n
			}
			found = true
		}
	}

	if !found {
		return psiTotals{}, errors.New("no total= in pressure file")
	}

	return totals, nil
}

// parseMeminfo returns MemAvailable and the swap in use, both in KiB.
func parseMeminfo(text string) (availableKiB, swapUsedKiB int64, err error) {
	values := map[string]int64{}

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		key, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		if n, parseErr := strconv.ParseInt(fields[0], 10, 64); parseErr == nil {
			values[key] = n
		}
	}

	available, ok := values["MemAvailable"]
	if !ok {
		return 0, 0, errors.New("meminfo has no MemAvailable")
	}

	return available, max(values["SwapTotal"]-values["SwapFree"], 0), nil
}

// between turns two readings into one interval's sample.
func between(prev, cur nodeReading) nodeSample {
	s := nodeSample{
		memAvailableMiB: float64(cur.memAvailableKiB) / 1024,
		swapUsedMiB:     float64(cur.swapUsedKiB) / 1024,
	}

	if total := cur.cpu.total() - prev.cpu.total(); total > 0 {
		pct := func(delta int64) float64 { return float64(delta) / float64(total) * 100 }

		s.stealPct = pct(cur.cpu.steal - prev.cpu.steal)
		s.iowaitPct = pct(cur.cpu.iowait - prev.cpu.iowait)
		// busy is everything but idle and waiting for the disk, steal included: the guest wanted to run
		s.busyPct = 100 - pct(cur.cpu.idle-prev.cpu.idle) - s.iowaitPct
	}

	if wall := cur.at.Sub(prev.at).Microseconds(); wall > 0 && len(cur.psi) == len(pressureResources) && len(prev.psi) == len(pressureResources) {
		stall := func(resource string, full bool) float64 {
			a, b := prev.psi[resource], cur.psi[resource]
			if full {
				return float64(b.full-a.full) / float64(wall) * 100
			}
			return float64(b.some-a.some) / float64(wall) * 100
		}

		s.hasPSI = true
		s.cpuSome = stall("cpu", false)
		s.memSome, s.memFull = stall("memory", false), stall("memory", true)
		s.ioSome, s.ioFull = stall("io", false), stall("io", true)
	}

	return s
}

var nodeHeader = []string{
	"rps", "samples", "busy_avg", "busy_p95", "steal_avg", "steal_max", "iowait_avg", "iowait_max",
	"psi_cpu_avg", "psi_cpu_max", "psi_mem_some_max", "psi_mem_full_max", "psi_io_some_avg", "psi_io_some_max",
	"psi_io_full_max", "mem_avail_min_mib", "swap_used_max_mib",
}

// nodeNA stands in for pressure numbers on a host without PSI.
const nodeNA = "na"

// nodeRecord is one run's row of the node CSV.
func nodeRecord(label string, samples []nodeSample) []string {
	column := func(value func(nodeSample) float64) []float64 {
		out := make([]float64, len(samples))
		for i, s := range samples {
			out[i] = value(s)
		}
		return out
	}
	hasPSI := samples[0].hasPSI
	psi := func(v []float64, summary func([]float64) float64) string {
		if !hasPSI {
			return nodeNA
		}
		return f1(summary(v))
	}
	maxOf := slices.Max[[]float64]

	busy := column(func(s nodeSample) float64 { return s.busyPct })
	steal := column(func(s nodeSample) float64 { return s.stealPct })
	iowait := column(func(s nodeSample) float64 { return s.iowaitPct })
	avail := column(func(s nodeSample) float64 { return s.memAvailableMiB })
	swap := column(func(s nodeSample) float64 { return s.swapUsedMiB })

	return []string{
		label, strconv.Itoa(len(samples)),
		f1(mean(busy)), f1(percentile(busy, 0.95)),
		f1(mean(steal)), f1(maxOf(steal)),
		f1(mean(iowait)), f1(maxOf(iowait)),
		psi(column(func(s nodeSample) float64 { return s.cpuSome }), mean),
		psi(column(func(s nodeSample) float64 { return s.cpuSome }), maxOf),
		psi(column(func(s nodeSample) float64 { return s.memSome }), maxOf),
		psi(column(func(s nodeSample) float64 { return s.memFull }), maxOf),
		psi(column(func(s nodeSample) float64 { return s.ioSome }), mean),
		psi(column(func(s nodeSample) float64 { return s.ioSome }), maxOf),
		psi(column(func(s nodeSample) float64 { return s.ioFull }), maxOf),
		f0(slices.Min(avail)), f0(maxOf(swap)),
	}
}

// nodePath is where a container CSV's host numbers go: summary.csv -> summary-node.csv.
func nodePath(out string) string {
	return strings.TrimSuffix(out, ".csv") + "-node.csv"
}

// reportNode prints the host table of a node CSV, or nothing if the file is not there.
func reportNode(path string) error {
	records, err := readCSV(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(records) < 2 {
		return nil
	}

	fmt.Println()
	fmt.Println("Whole host (percentages of wall time or of all CPU time; PSI = share of time work waited):")
	fmt.Println()
	fmt.Printf("%6s %9s %9s %9s %9s %9s %9s %9s %9s %9s %10s %9s\n",
		"rps", "busy avg", "busy p95", "steal avg", "steal max", "iowait mx", "cpu psi", "mem psi f", "io psi", "io psi f", "mem avail", "swap max")
	for _, rec := range records[1:] {
		// rps busy_avg busy_p95 steal_avg steal_max iowait_max psi_cpu_avg psi_mem_full_max psi_io_some_avg psi_io_full_max mem_avail swap
		fmt.Printf("%6s %9s %9s %9s %9s %9s %9s %9s %9s %9s %10s %9s\n",
			rec[0], rec[2], rec[3], rec[4], rec[5], rec[7], rec[8], rec[11], rec[12], rec[14], rec[15]+" MiB", rec[16]+" MiB")
	}

	return nil
}
