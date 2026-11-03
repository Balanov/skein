package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
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
	Candidate        string   `json:"candidate"`
	Tasks            int      `json:"tasks"`
	Shape            string   `json:"shape"`
	Workers          int      `json:"workers"`
	InnerRepetitions int      `json:"inner_repetitions"`
	GoVersion        string   `json:"go_version"`
	Samples          []sample `json:"samples"`
}

func main() {
	candidate := flag.String("candidate", "skein", "candidate: skein or go")
	tasks := flag.Int("tasks", 1000, "number of no-op graph nodes")
	shape := flag.String("shape", "independent", "graph shape: independent, chain or fanout")
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "Skein worker limit")
	runs := flag.Int("runs", 5, "measured runs")
	warmups := flag.Int("warmups", 2, "warmup runs")
	flag.Parse()
	if *tasks < 1 || *workers < 1 || *runs < 1 || *warmups < 0 || (*candidate != "skein" && *candidate != "go") || (*shape != "independent" && *shape != "chain" && *shape != "fanout") || (*shape == "fanout" && *tasks < 3) {
		fmt.Fprintln(os.Stderr, "invalid benchmark arguments")
		os.Exit(2)
	}
	name := "skein"
	repetitions := 1
	if *candidate == "go" {
		name = "go-goroutines"
		repetitions = 10
	}
	result := output{Candidate: name, Tasks: *tasks, Shape: *shape, Workers: *workers, InnerRepetitions: repetitions, GoVersion: runtime.Version(), Samples: make([]sample, 0, *runs)}
	state, err := os.MkdirTemp("", "skein-bench-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(state)

	var manifest engine.Manifest
	executor := engine.ExecutorFunc(func(ctx context.Context, lease engine.Lease, task engine.Task, block engine.Block, stdin, stdout string) error {
		return nil
	})
	if *candidate == "skein" {
		manifest = buildManifest(*tasks, *workers, *shape)
	}
	for iteration := 0; iteration < *warmups+*runs; iteration++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		started := time.Now()
		succeeded := *tasks
		if *candidate == "skein" {
			report, runErr := engine.Run(context.Background(), manifest, engine.Options{StateDir: state, Executor: executor, InMemory: true})
			if runErr != nil || report.Succeeded != *tasks {
				fmt.Fprintf(os.Stderr, "benchmark failed: report=%+v err=%v\n", report, runErr)
				os.Exit(1)
			}
		} else {
			for repetition := 0; repetition < repetitions; repetition++ {
				runGoBaseline(*tasks, *shape)
			}
		}
		elapsed := time.Since(started) / time.Duration(repetitions)
		if elapsed <= 0 {
			elapsed = 1
		}
		runtime.ReadMemStats(&after)
		if iteration >= *warmups {
			result.Samples = append(result.Samples, sample{Run: iteration - *warmups + 1, DurationNS: elapsed.Nanoseconds(), TasksPerSecond: float64(*tasks) / elapsed.Seconds(), AllocatedBytes: after.TotalAlloc - before.TotalAlloc, AllocationCount: after.Mallocs - before.Mallocs, Succeeded: succeeded})
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}

func buildManifest(tasks, workers int, shape string) engine.Manifest {
	manifest := engine.Manifest{Version: 1, Workers: workers, Blocks: map[string]engine.Block{"noop": {Argv: []string{"noop"}}}, Tasks: make([]engine.Task, tasks)}
	for index := range manifest.Tasks {
		manifest.Tasks[index] = engine.Task{ID: taskID(index), Block: "noop"}
	}
	switch shape {
	case "chain":
		for index := 1; index < tasks; index++ {
			manifest.Tasks[index].Needs = []string{taskID(index - 1)}
		}
	case "fanout":
		for index := 1; index < tasks-1; index++ {
			manifest.Tasks[index].Needs = []string{taskID(0)}
			manifest.Tasks[tasks-1].Needs = append(manifest.Tasks[tasks-1].Needs, taskID(index))
		}
	}
	return manifest
}

func taskID(index int) string {
	return fmt.Sprintf("task-%06d", index)
}

// runGoBaseline is an unstructured lower-bound control, not another orchestrator.
// It performs no validation, leases, result reports, audit events or fencing.
func runGoBaseline(tasks int, shape string) {
	switch shape {
	case "independent":
		var work sync.WaitGroup
		work.Add(tasks)
		for index := 0; index < tasks; index++ {
			go func() { defer work.Done() }()
		}
		work.Wait()
	case "chain":
		var previous chan struct{}
		for index := 0; index < tasks; index++ {
			done := make(chan struct{})
			prior := previous
			go func() {
				if prior != nil {
					<-prior
				}
				close(done)
			}()
			previous = done
		}
		<-previous
	case "fanout":
		source := make(chan struct{})
		go func() { close(source) }()
		var leaves sync.WaitGroup
		leaves.Add(tasks - 2)
		for index := 1; index < tasks-1; index++ {
			go func() {
				<-source
				leaves.Done()
			}()
		}
		sink := make(chan struct{})
		go func() {
			leaves.Wait()
			close(sink)
		}()
		<-sink
	}
}
