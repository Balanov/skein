package engine

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkIndependentScheduler(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("tasks=%d", size), func(b *testing.B) {
			manifest := Manifest{Version: 1, Workers: 256, Blocks: map[string]Block{"noop": {Argv: []string{"noop"}}}, Tasks: make([]Task, size)}
			for index := range manifest.Tasks {
				manifest.Tasks[index] = Task{ID: fmt.Sprintf("task-%06d", index), Block: "noop"}
			}
			executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error { return nil })
			state := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				report, err := Run(context.Background(), manifest, Options{StateDir: state, Executor: executor, InMemory: true})
				if err != nil || report.Succeeded != size {
					b.Fatalf("report=%+v err=%v", report, err)
				}
			}
			b.ReportMetric(float64(size*b.N)/b.Elapsed().Seconds(), "tasks/s")
		})
	}
}
