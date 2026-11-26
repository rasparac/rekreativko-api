package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCPUStat(t *testing.T) {
	text := "cpu  100 5 50 800 20 3 2 20 0 0\ncpu0 50 2 25 400 10 1 1 10 0 0\nintr 12345\n"

	got, err := parseCPUStat(text)

	require.NoError(t, err)
	assert.Equal(t, cpuTimes{user: 100, nice: 5, system: 50, idle: 800, iowait: 20, irq: 3, softirq: 2, steal: 20}, got)
	assert.Equal(t, int64(1000), got.total())

	_, err = parseCPUStat("intr 1\n")
	assert.Error(t, err, "no aggregate cpu line")

	_, err = parseCPUStat("cpu 1 2 3\n")
	assert.Error(t, err, "too few fields")
}

func TestParsePressure(t *testing.T) {
	t.Run("some and full", func(t *testing.T) {
		got, err := parsePressure("some avg10=0.50 avg60=0.20 avg300=0.10 total=123456\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=789\n")

		require.NoError(t, err)
		assert.Equal(t, psiTotals{some: 123456, full: 789}, got)
	})

	t.Run("a file with only some, as the cpu one can be", func(t *testing.T) {
		got, err := parsePressure("some avg10=0.00 avg60=0.00 avg300=0.00 total=42\n")

		require.NoError(t, err)
		assert.Equal(t, psiTotals{some: 42}, got)
	})

	t.Run("nothing to read", func(t *testing.T) {
		_, err := parsePressure("garbage\n")
		assert.Error(t, err)
	})
}

func TestParseMeminfo(t *testing.T) {
	text := "MemTotal:        8000000 kB\nMemFree:          100000 kB\nMemAvailable:    2500000 kB\nSwapTotal:       1000000 kB\nSwapFree:         400000 kB\n"

	available, swapUsed, err := parseMeminfo(text)

	require.NoError(t, err)
	assert.Equal(t, int64(2500000), available)
	assert.Equal(t, int64(600000), swapUsed)

	_, _, err = parseMeminfo("MemTotal: 1 kB\n")
	assert.Error(t, err, "no MemAvailable")
}

func nodeAt(at time.Time, cpu cpuTimes, psi map[string]psiTotals) nodeReading {
	return nodeReading{at: at, cpu: cpu, psi: psi, memAvailableKiB: 2048 * 1024, swapUsedKiB: 512 * 1024}
}

func fullPSI(cpu, memSome, memFull, ioSome, ioFull int64) map[string]psiTotals {
	return map[string]psiTotals{"cpu": {some: cpu}, "memory": {some: memSome, full: memFull}, "io": {some: ioSome, full: ioFull}}
}

func TestBetween_CPUAndPressure(t *testing.T) {
	start := time.Unix(1000, 0)
	prev := nodeAt(start, cpuTimes{user: 100, idle: 900}, fullPSI(0, 0, 0, 0, 0))
	// 1000 jiffies passed: 400 busy (user 350 + system 50), 450 idle, 100 waiting for the disk, 50 stolen
	cur := nodeAt(start.Add(time.Second), cpuTimes{user: 450, system: 50, idle: 1350, iowait: 100, steal: 50}, fullPSI(250_000, 100_000, 40_000, 500_000, 200_000))

	got := between(prev, cur)

	assert.InDelta(t, 5.0, got.stealPct, 0.01)
	assert.InDelta(t, 10.0, got.iowaitPct, 0.01)
	assert.InDelta(t, 45.0, got.busyPct, 0.01, "everything but idle and iowait, steal included")
	require.True(t, got.hasPSI)
	assert.InDelta(t, 25.0, got.cpuSome, 0.01, "250 ms of a 1 s interval")
	assert.InDelta(t, 10.0, got.memSome, 0.01)
	assert.InDelta(t, 4.0, got.memFull, 0.01)
	assert.InDelta(t, 50.0, got.ioSome, 0.01)
	assert.InDelta(t, 20.0, got.ioFull, 0.01)
	assert.InDelta(t, 2048.0, got.memAvailableMiB, 0.01)
	assert.InDelta(t, 512.0, got.swapUsedMiB, 0.01)
}

func TestBetween_HostWithoutPSI(t *testing.T) {
	start := time.Unix(1000, 0)
	prev := nodeAt(start, cpuTimes{idle: 100}, map[string]psiTotals{})
	cur := nodeAt(start.Add(time.Second), cpuTimes{user: 50, idle: 150}, map[string]psiTotals{})

	got := between(prev, cur)

	assert.False(t, got.hasPSI)
	assert.InDelta(t, 50.0, got.busyPct, 0.01)
}

func TestNodeRecord(t *testing.T) {
	samples := []nodeSample{
		{busyPct: 20, stealPct: 1, iowaitPct: 2, cpuSome: 3, memSome: 4, memFull: 1, ioSome: 5, ioFull: 2, hasPSI: true, memAvailableMiB: 3000, swapUsedMiB: 0},
		{busyPct: 60, stealPct: 9, iowaitPct: 6, cpuSome: 9, memSome: 8, memFull: 3, ioSome: 15, ioFull: 6, hasPSI: true, memAvailableMiB: 2500, swapUsedMiB: 100},
	}

	rec := nodeRecord("100", samples)

	require.Len(t, rec, len(nodeHeader))
	row := map[string]string{}
	for i, name := range nodeHeader {
		row[name] = rec[i]
	}
	assert.Equal(t, "100", row["rps"])
	assert.Equal(t, "2", row["samples"])
	assert.Equal(t, "40.0", row["busy_avg"])
	assert.Equal(t, "5.0", row["steal_avg"])
	assert.Equal(t, "9.0", row["steal_max"])
	assert.Equal(t, "9.0", row["psi_cpu_max"])
	assert.Equal(t, "3.0", row["psi_mem_full_max"])
	assert.Equal(t, "2500", row["mem_avail_min_mib"], "the lowest, not the average")
	assert.Equal(t, "100", row["swap_used_max_mib"])

	t.Run("no PSI says so", func(t *testing.T) {
		rec := nodeRecord("0", []nodeSample{{busyPct: 1, memAvailableMiB: 1000}})
		row := map[string]string{}
		for i, name := range nodeHeader {
			row[name] = rec[i]
		}
		assert.Equal(t, nodeNA, row["psi_cpu_avg"])
		assert.Equal(t, nodeNA, row["psi_io_full_max"])
		assert.Equal(t, "1.0", row["busy_avg"])
	})
}

func TestNodePath(t *testing.T) {
	assert.Equal(t, "loadtest/results/x/summary-node.csv", nodePath("loadtest/results/x/summary.csv"))
}
