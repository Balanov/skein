package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testManifest(tasks ...Task) Manifest {
	return Manifest{Version: 1, Workers: 2, Blocks: map[string]Block{"work": {Argv: []string{"unused"}}}, Tasks: tasks}
}

func TestValidateRejectsUnsafeGraphs(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		contains string
	}{
		{"cycle", testManifest(Task{ID: "a", Block: "work", Needs: []string{"b"}}, Task{ID: "b", Block: "work", Needs: []string{"a"}}), "cycle"},
		{"duplicate", testManifest(Task{ID: "a", Block: "work"}, Task{ID: "a", Block: "work"}), "duplicate"},
		{"unknown dependency", testManifest(Task{ID: "a", Block: "work", Needs: []string{"missing"}}), "unknown"},
		{"un declared input", testManifest(Task{ID: "a", Block: "work"}, Task{ID: "b", Block: "work", StdinFrom: "a"}), "without declaring"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.manifest)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.contains)
			}
		})
	}
}

func TestValidateLimits(t *testing.T) {
	manifest := testManifest(Task{ID: "a", Block: "work", Passes: 3}, Task{ID: "b", Block: "work"})
	manifest.Limits.MaxTasks = 1
	if err := Validate(manifest); err == nil || !strings.Contains(err.Error(), "task limit") {
		t.Fatalf("expected task limit error, got %v", err)
	}
	manifest.Limits.MaxTasks = 0
	manifest.Limits.MaxPasses = 2
	if err := Validate(manifest); err == nil || !strings.Contains(err.Error(), "pass limit") {
		t.Fatalf("expected pass limit error, got %v", err)
	}
}

func TestSharedBlockLoopsAndPassesData(t *testing.T) {
	manifest := testManifest(
		Task{ID: "a", Block: "work", Input: "start", Passes: 2},
		Task{ID: "b", Block: "work", Needs: []string{"a"}, StdinFrom: "a"},
	)
	executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
		data, err := os.ReadFile(stdin)
		if err != nil {
			return err
		}
		return os.WriteFile(stdout, append(data, []byte(fmt.Sprintf(":%s/%d", task.ID, lease.Pass))...), 0o600)
	})
	state := t.TempDir()
	report, err := Run(context.Background(), manifest, Options{StateDir: state, Executor: executor, Events: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Succeeded || report.Tasks["a"].Passes != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	data, err := os.ReadFile(report.Tasks["b"].OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "start:a/1:a/2:b/1"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	events, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(report.Tasks["b"].OutputPath)), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(events), "start:a") {
		t.Fatal("payload leaked into control event stream")
	}
}

func TestFailureSkipsOnlyDependents(t *testing.T) {
	manifest := testManifest(
		Task{ID: "bad", Block: "work"},
		Task{ID: "child", Block: "work", Needs: []string{"bad"}},
		Task{ID: "independent", Block: "work"},
	)
	executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
		if task.ID == "bad" {
			return errors.New("intentional failure")
		}
		return os.WriteFile(stdout, []byte("ok"), 0o600)
	})
	report, err := Run(context.Background(), manifest, Options{StateDir: t.TempDir(), Executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	if report.Tasks["bad"].Status != Failed || report.Tasks["child"].Status != Skipped || report.Tasks["independent"].Status != Succeeded {
		t.Fatalf("unexpected statuses: %+v", report.Tasks)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	tasks := make([]Task, 24)
	for index := range tasks {
		tasks[index] = Task{ID: fmt.Sprintf("t-%02d", index), Block: "work"}
	}
	manifest := testManifest(tasks...)
	manifest.Workers = 3
	var active atomic.Int32
	var peak atomic.Int32
	executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		return nil
	})
	report, err := Run(context.Background(), manifest, Options{StateDir: t.TempDir(), Executor: executor, InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Succeeded != len(tasks) || peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("succeeded=%d peak=%d", report.Succeeded, peak.Load())
	}
}

func TestStopFencesQueuedWorkAndResetKeepsEpoch(t *testing.T) {
	manifest := testManifest(Task{ID: "running", Block: "work"}, Task{ID: "queued", Block: "work"})
	manifest.Workers = 1
	started := make(chan struct{})
	executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	state := t.TempDir()
	result := make(chan Report, 1)
	errResult := make(chan error, 1)
	go func() {
		report, err := Run(context.Background(), manifest, Options{StateDir: state, Executor: executor, PollInterval: time.Millisecond})
		result <- report
		errResult <- err
	}()
	<-started
	if err := Stop(state, "test stop"); err != nil {
		t.Fatal(err)
	}
	if err := Stop(state, "duplicate stop"); err != nil {
		t.Fatal(err)
	}
	report := <-result
	if err := <-errResult; err != nil {
		t.Fatal(err)
	}
	if report.Status != Stopped || report.Succeeded != 0 || report.Tasks["queued"].Status != Stopped {
		t.Fatalf("stop did not fence queue: %+v", report)
	}
	epoch, err := CurrentEpoch(state)
	if err != nil || epoch != 2 {
		t.Fatalf("epoch=%d err=%v", epoch, err)
	}
	if _, err := Run(context.Background(), manifest, Options{StateDir: state, Executor: executor}); !errors.Is(err, ErrStopped) {
		t.Fatalf("new run while stopped: %v", err)
	}
	if err := Reset(state, false); err != nil {
		t.Fatal(err)
	}
	epoch, err = CurrentEpoch(state)
	if err != nil || epoch != 2 {
		t.Fatalf("reset reused epoch=%d err=%v", epoch, err)
	}
}

func TestLateCompletionCannotReleaseDependents(t *testing.T) {
	manifest := testManifest(Task{ID: "late", Block: "work"}, Task{ID: "child", Block: "work", Needs: []string{"late"}})
	manifest.Workers = 1
	started := make(chan struct{})
	release := make(chan struct{})
	executor := ExecutorFunc(func(ctx context.Context, lease Lease, task Task, block Block, stdin, stdout string) error {
		close(started)
		<-release // Deliberately late: completion races past cancellation.
		return os.WriteFile(stdout, []byte("stale result"), 0o600)
	})
	state := t.TempDir()
	done := make(chan Report, 1)
	go func() {
		report, _ := Run(context.Background(), manifest, Options{StateDir: state, Executor: executor, PollInterval: time.Millisecond})
		done <- report
	}()
	<-started
	if err := Stop(state, "fence late result"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	report := <-done
	if report.Tasks["late"].Status != Stopped || report.Tasks["child"].Status != Stopped || report.Succeeded != 0 {
		t.Fatalf("late completion was accepted: %+v", report.Tasks)
	}
}

func TestFencingTokenValidation(t *testing.T) {
	lease := Lease{RunID: "run", Epoch: 7, Sequence: 1, TaskID: "task", Pass: 1}
	if !LeaseValid(lease, 7) {
		t.Fatal("valid lease rejected")
	}
	if LeaseValid(lease, 8) {
		t.Fatal("stale lease accepted")
	}
}
