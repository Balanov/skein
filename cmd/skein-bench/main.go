package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Balanov/skein/internal/engine"
)

type sample struct {
	Run             int     `json:"run"`
	DurationNS      int64   `json:"duration_ns"`
	TasksPerSecond  float64 `json:"tasks_per_second"`
	AllocatedBytes  uint64  `json:"allocated_bytes"`
	AllocationCount uint64  `json:"allocation_count"`
	Succeeded       int     `json:"succeeded"`
}

type output struct {
	Candidate string   `json:"candidate"`
	Tasks     int      `json:"tasks"`
	Workers   int      `json:"workers"`
	GoVersion string   `json:"go_version"`
	Samples   []sample `json:"samples"`
}

func main() {
	tasks := flag.Int("tasks", 1000, "independent no-op tasks")
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "concurrent tasks")
	runs := flag.Int("runs", 9, "measured runs")
	warmups := flag.Int("warmups", 2, "warmup runs")
	flag.Parse()
	if *tasks < 1 || *workers < 1 || *runs < 1 || *warmups < 0 {
		fmt.Fprintln(os.Stderr, "invalid benchmark arguments")
		os.Exit(2)
	}
	manifest := engine.Manifest{Version: 1, Workers: *workers, Blocks: map[string]engine.Block{"noop": {Argv: []string{"noop"}}}, Tasks: make([]engine.Task, *tasks)}
	for index := range manifest.Tasks {
		manifest.Tasks[index] = engine.Task{ID: fmt.Sprintf("task-%06d", index), Block: "noop"}
	}
	executor := engine.ExecutorFunc(func(ctx context.Context, lease engine.Lease, task engine.Task, block engine.Block, stdin, stdout string) error {
		return nil
	})
	result := output{Candidate: "skein", Tasks: *tasks, Workers: *workers, GoVersion: runtime.Version(), Samples: make([]sample, 0, *runs)}
	state, err := os.MkdirTemp("", "skein-bench-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(state)
	for iteration := 0; iteration < *warmups+*runs; iteration++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		started := time.Now()
		report, err := engine.Run(context.Background(), manifest, engine.Options{StateDir: state, Executor: executor, InMemory: true})
		elapsed := time.Since(started)
		runtime.ReadMemStats(&after)
		if err != nil || report.Succeeded != *tasks {
			fmt.Fprintf(os.Stderr, "benchmark failed: report=%+v err=%v\n", report, err)
			os.Exit(1)
		}
		if iteration >= *warmups {
			result.Samples = append(result.Samples, sample{Run: iteration - *warmups + 1, DurationNS: elapsed.Nanoseconds(), TasksPerSecond: float64(*tasks) / elapsed.Seconds(), AllocatedBytes: after.TotalAlloc - before.TotalAlloc, AllocationCount: after.Mallocs - before.Mallocs, Succeeded: report.Succeeded})
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}
