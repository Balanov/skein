package engine

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

func TestDeterministicRandomDAGs(t *testing.T) {
	random := rand.New(rand.NewSource(20261103))
	const graphs = 128
	totalTasks := 0
	for graphIndex := 0; graphIndex < graphs; graphIndex++ {
		t.Run(fmt.Sprintf("graph=%03d", graphIndex), func(t *testing.T) {
			size := 1 + random.Intn(96)
			workers := 1 + random.Intn(8)
			manifest := Manifest{Version: 1, Workers: workers, Blocks: map[string]Block{"noop": {Argv: []string{"unused"}}}, Tasks: make([]Task, size)}
			for index := range manifest.Tasks {
				task := Task{ID: fmt.Sprintf("task-%03d", index), Block: "noop"}
				for dependency := 0; dependency < index; dependency++ {
					if random.Intn(12) == 0 {
						task.Needs = append(task.Needs, manifest.Tasks[dependency].ID)
					}
				}
				manifest.Tasks[index] = task
			}
			var mutex sync.Mutex
			finished := make(map[string]bool, size)
			active, peak := 0, 0
			executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
				mutex.Lock()
				for _, need := range task.Needs {
					if !finished[need] {
						mutex.Unlock()
						return fmt.Errorf("%s started before dependency %s finished", task.ID, need)
					}
				}
				active++
				if active > peak {
					peak = active
				}
				mutex.Unlock()
				time.Sleep(20 * time.Microsecond)
				mutex.Lock()
				active--
				finished[task.ID] = true
				mutex.Unlock()
				return nil
			})
			report, err := Run(context.Background(), manifest, Options{StateDir: t.TempDir(), Executor: executor, InMemory: true})
			if err != nil || report.Succeeded != size {
				t.Fatalf("size=%d workers=%d report=%+v err=%v", size, workers, report, err)
			}
			if peak > workers {
				t.Fatalf("worker bound exceeded: peak=%d workers=%d", peak, workers)
			}
			totalTasks += size
		})
	}
	t.Logf("verified %d deterministic random DAGs containing %d tasks", graphs, totalTasks)
}

func TestConcurrentStopStorm(t *testing.T) {
	const rounds = 25
	const callers = 32
	for round := 0; round < rounds; round++ {
		state := t.TempDir()
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		errors := make(chan error, callers)
		var callersWait sync.WaitGroup
		for caller := 0; caller < callers; caller++ {
			callersWait.Add(1)
			go func(caller int) {
				defer callersWait.Done()
				<-start
				errors <- Stop(state, fmt.Sprintf("round=%d caller=%d", round, caller))
			}(caller)
		}
		close(start)
		callersWait.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
		before, err := os.ReadFile(StopPath(state))
		if err != nil {
			t.Fatal(err)
		}
		if err := Stop(state, "duplicate"); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(StopPath(state))
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("duplicate stop replaced the first latch")
		}
		epoch, err := CurrentEpoch(state)
		if err != nil || epoch != 2 {
			t.Fatalf("round=%d epoch=%d err=%v", round, epoch, err)
		}
	}
	t.Logf("verified %d concurrent stop calls across %d rounds", rounds*callers, rounds)
}
