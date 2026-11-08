// Command resource-sampler records how much CPU and memory docker containers
// use while a load test runs, the way Kubernetes would account for them, to
// size each service's requests and limits (loadtest/README.md).
//
// It reads the containers' cgroup v2 files directly (Linux, docker with the
// systemd cgroup driver) rather than `docker stats`, which is slow to poll and
// reports memory including the reclaimable page cache:
//
//   - CPU is the change in cpu.stat usage_usec over wall time, in millicores.
//   - Memory is the working set - memory.current minus inactive_file - which
//     is what the kubelet compares against a memory limit and what an OOM
//     kill is decided on.
//
// Sample until interrupted, then append one row per container to a CSV:
//
//	go run ./scripts/resource-sampler -label 50 -out loadtest/results/summary.csv
//
// Print the table and a per-service fit of CPU against request rate, from
// every run in the CSV (the label is the run's request rate, 0 = idle):
//
//	go run ./scripts/resource-sampler -report loadtest/results/summary.csv -fit-max-rps 400
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultContainers = "gateway,identity,account-profile,activity,notifications,location,outbox-publisher,pgbouncer,rekreativko_db,rekreativko_nats,rekreativko_redis"

type options struct {
	containers []string
	interval   time.Duration
	skip       time.Duration
	label      string
	out        string
	report     string
	fitMaxRPS  float64
}

func main() {
	var opts options
	var containers string
	flag.StringVar(&containers, "containers", defaultContainers, "comma-separated container names to sample")
	flag.DurationVar(&opts.interval, "interval", time.Second, "time between samples")
	flag.DurationVar(&opts.skip, "skip", 10*time.Second, "leave out the first samples (the load generator logging in and warming up)")
	flag.StringVar(&opts.label, "label", "", "the run's request rate per second, 0 for idle (required when sampling)")
	flag.StringVar(&opts.out, "out", "", "CSV to append the summary to (required when sampling)")
	flag.StringVar(&opts.report, "report", "", "print the report for this CSV instead of sampling")
	flag.Float64Var(&opts.fitMaxRPS, "fit-max-rps", 0, "fit CPU only on runs up to this rate, where it is still linear (0 = every run)")
	flag.Parse()
	opts.containers = strings.Split(containers, ",")

	var err error
	if opts.report != "" {
		err = report(opts.report, opts.fitMaxRPS)
	} else {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = sample(ctx, opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// reading is one container's counters at one moment.
type reading struct {
	at         time.Time
	cpuUsec    int64
	workingSet int64
}

// series is what one container used over the run.
type series struct {
	name       string
	cgroup     string
	last       reading
	millicores []float64
	workingSet []int64
}

func sample(ctx context.Context, opts options) error {
	if opts.label == "" || opts.out == "" {
		return errors.New("-label and -out are required when sampling")
	}
	if _, err := strconv.ParseFloat(opts.label, 64); err != nil {
		return fmt.Errorf("-label must be the request rate: %w", err)
	}

	all := make([]*series, 0, len(opts.containers))
	for _, name := range opts.containers {
		dir, err := cgroupDir(ctx, name)
		if err != nil {
			return err
		}
		s := &series{name: name, cgroup: dir}
		if s.last, err = read(dir); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		all = append(all, s)
	}

	started := time.Now()
	ticker := time.NewTicker(opts.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return summarize(opts, all)
		case <-ticker.C:
		}

		keep := time.Since(started) >= opts.skip
		for _, s := range all {
			r, err := read(s.cgroup)
			if err != nil {
				// the container went away (restarted, OOM-killed) - say so
				// rather than report numbers that stop short
				return fmt.Errorf("%s: %w", s.name, err)
			}
			if keep {
				wall := r.at.Sub(s.last.at).Microseconds()
				s.millicores = append(s.millicores, float64(r.cpuUsec-s.last.cpuUsec)/float64(wall)*1000)
				s.workingSet = append(s.workingSet, r.workingSet)
			}
			s.last = r
		}
	}
}

func cgroupDir(ctx context.Context, container string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Id}}", container).Output()
	if err != nil {
		return "", fmt.Errorf("find container %s: %w", container, err)
	}
	dir := filepath.Join("/sys/fs/cgroup/system.slice", "docker-"+strings.TrimSpace(string(out))+".scope")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("container %s has no cgroup v2 directory (systemd cgroup driver expected): %w", container, err)
	}
	return dir, nil
}

func read(dir string) (reading, error) {
	r := reading{at: time.Now()}

	cpu, err := statValue(filepath.Join(dir, "cpu.stat"), "usage_usec")
	if err != nil {
		return reading{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "memory.current"))
	if err != nil {
		return reading{}, err
	}
	current, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return reading{}, err
	}
	inactiveFile, err := statValue(filepath.Join(dir, "memory.stat"), "inactive_file")
	if err != nil {
		return reading{}, err
	}

	r.cpuUsec = cpu
	r.workingSet = max(current-inactiveFile, 0)
	return r, nil
}

// statValue reads one "key value" line of a cgroup stat file.
func statValue(path, key string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), " ")
		if ok && k == key {
			return strconv.ParseInt(v, 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("%s has no %s", path, key)
}

var header = []string{"rps", "container", "samples", "cpu_avg_m", "cpu_p95_m", "cpu_max_m", "mem_avg_mib", "mem_max_mib"}

func summarize(opts options, all []*series) error {
	if err := os.MkdirAll(filepath.Dir(opts.out), 0o755); err != nil {
		return err
	}
	_, statErr := os.Stat(opts.out)
	f, err := os.OpenFile(opts.out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if errors.Is(statErr, os.ErrNotExist) {
		_ = w.Write(header)
	}
	for _, s := range all {
		if len(s.millicores) == 0 {
			return fmt.Errorf("no samples kept: the run was shorter than -skip %s", opts.skip)
		}
		mem := make([]float64, len(s.workingSet))
		for i, b := range s.workingSet {
			mem[i] = float64(b) / (1 << 20)
		}
		_ = w.Write([]string{
			opts.label, s.name, strconv.Itoa(len(s.millicores)),
			f0(mean(s.millicores)), f0(percentile(s.millicores, 0.95)), f0(slices.Max(s.millicores)),
			f1(mean(mem)), f1(slices.Max(mem)),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	fmt.Printf("sampled %d containers at %s rps, appended to %s\n", len(all), opts.label, opts.out)
	return nil
}

// row is one container in one run of the CSV.
type row struct {
	rps                    float64
	container              string
	cpuAvg, cpuP95, cpuMax float64
	memAvg, memMax         float64
}

func report(path string, fitMaxRPS float64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}
	if len(records) < 2 {
		return fmt.Errorf("%s has no runs", path)
	}

	var rows []row
	var order []string
	seen := map[string]struct{}{}
	for _, rec := range records[1:] {
		num := func(i int) float64 {
			v, _ := strconv.ParseFloat(rec[i], 64)
			return v
		}
		r := row{rps: num(0), container: rec[1], cpuAvg: num(3), cpuP95: num(4), cpuMax: num(5), memAvg: num(6), memMax: num(7)}
		rows = append(rows, r)
		if _, ok := seen[r.container]; !ok {
			seen[r.container] = struct{}{}
			order = append(order, r.container)
		}
	}

	fmt.Println("Per run (CPU in millicores, memory = working set in MiB):")
	fmt.Println()
	fmt.Printf("%-18s %6s %8s %8s %8s %8s %8s\n", "container", "rps", "cpu avg", "cpu p95", "cpu max", "mem avg", "mem max")
	for _, name := range order {
		for _, r := range rows {
			if r.container == name {
				fmt.Printf("%-18s %6.0f %8.0f %8.0f %8.0f %8.1f %8.1f\n", name, r.rps, r.cpuAvg, r.cpuP95, r.cpuMax, r.memAvg, r.memMax)
			}
		}
	}

	// CPU grows about linearly with the request rate until something
	// saturates, so idle + slope x rate predicts the CPU at any rate - the
	// basis for the requests (loadtest/README.md).
	fmt.Println()
	if fitMaxRPS > 0 {
		fmt.Printf("Least-squares fit of average CPU against request rate, runs up to %.0f rps:\n", fitMaxRPS)
	} else {
		fmt.Println("Least-squares fit of average CPU against request rate:")
	}
	fmt.Println()
	fmt.Printf("%-18s %10s %16s %12s\n", "container", "idle (m)", "m per 100 rps", "max mem MiB")
	for _, name := range order {
		var xs, ys []float64
		maxMem := 0.0
		for _, r := range rows {
			if r.container != name {
				continue
			}
			maxMem = max(maxMem, r.memMax)
			if fitMaxRPS == 0 || r.rps <= fitMaxRPS {
				xs = append(xs, r.rps)
				ys = append(ys, r.cpuAvg)
			}
		}
		intercept, slope := fit(xs, ys)
		fmt.Printf("%-18s %10.0f %16.0f %12.1f\n", name, intercept, slope*100, maxMem)
	}
	return nil
}

func fit(xs, ys []float64) (intercept, slope float64) {
	mx, my := mean(xs), mean(ys)
	var num, den float64
	for i := range xs {
		num += (xs[i] - mx) * (ys[i] - my)
		den += (xs[i] - mx) * (xs[i] - mx)
	}
	if den == 0 {
		return my, 0
	}
	slope = num / den
	return my - slope*mx, slope
}

func mean(xs []float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func percentile(xs []float64, p float64) float64 {
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	return sorted[int(p*float64(len(sorted)-1))]
}

func f0(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
