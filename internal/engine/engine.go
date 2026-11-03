package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
)

var ErrStopped = errors.New("swarm stopped")

type Options struct {
	StateDir     string
	Workers      int
	Executor     Executor
	Events       bool
	SyncEvents   bool
	IncludeTasks bool
	PollInterval time.Duration
	// InMemory is reserved for scheduler benchmarks. It skips process artifacts,
	// lease files, locks and stop-file polling while retaining fencing checks.
	InMemory bool
}

type taskState struct {
	task      Task
	remaining int
	status    Status
	result    TaskResult
}

type taskExecution struct {
	id     string
	lease  Lease
	passes int
	output string
	err    error
}

type scheduler struct {
	manifest   Manifest
	options    Options
	runID      string
	runDir     string
	epoch      uint64
	workers    int
	executor   Executor
	logger     *eventLogger
	sequence   atomic.Uint64
	tasks      map[string]*taskState
	dependents map[string][]string
	ready      chan string
	done       chan taskExecution
	running    int
	finished   int
	stopped    atomic.Bool
}

func Run(ctx context.Context, manifest Manifest, options Options) (Report, error) {
	started := time.Now().UTC()
	if err := Validate(manifest); err != nil {
		return Report{}, err
	}
	if options.StateDir == "" {
		options.StateDir = ".skein"
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 10 * time.Millisecond
	}
	if options.Executor == nil {
		options.Executor = ProcessExecutor{}
	}
	if err := os.MkdirAll(options.StateDir, 0o700); err != nil {
		return Report{}, err
	}
	if !options.InMemory && StopRequested(options.StateDir) {
		return Report{}, ErrStopped
	}
	epoch, err := CurrentEpoch(options.StateDir)
	if err != nil {
		return Report{}, err
	}
	runID, err := newRunID()
	if err != nil {
		return Report{}, err
	}
	runDir := filepath.Join(options.StateDir, "runs", runID)
	if err := os.MkdirAll(filepath.Join(runDir, "data"), 0o700); err != nil {
		return Report{}, err
	}
	if !options.InMemory {
		if err := writeEpoch(options.StateDir, epoch); err != nil {
			return Report{}, err
		}
	}
	lock, err := acquireRunLock(options, runID, epoch)
	if err != nil {
		return Report{}, err
	}
	if lock != nil {
		defer func() {
			lock.Close()
			os.Remove(lock.Name())
		}()
	}

	scheduler := &scheduler{
		manifest: manifest, options: options, runID: runID, runDir: runDir,
		epoch: epoch, workers: WorkerCount(manifest, options.Workers), executor: options.Executor,
		tasks: make(map[string]*taskState, len(manifest.Tasks)), dependents: make(map[string][]string, len(manifest.Tasks)),
		ready: make(chan string, len(manifest.Tasks)), done: make(chan taskExecution, WorkerCount(manifest, options.Workers)),
	}
	if scheduler.workers < 1 {
		scheduler.workers = runtime.GOMAXPROCS(0)
	}
	if options.Events {
		scheduler.logger, err = openEventLogger(filepath.Join(runDir, "events.jsonl"), options.SyncEvents)
		if err != nil {
			return Report{}, err
		}
	}
	for _, task := range manifest.Tasks {
		scheduler.tasks[task.ID] = &taskState{task: task, remaining: len(task.Needs), status: Pending, result: TaskResult{ID: task.ID, Status: Pending}}
		for _, need := range task.Needs {
			scheduler.dependents[need] = append(scheduler.dependents[need], task.ID)
		}
		if len(task.Needs) == 0 {
			scheduler.ready <- task.ID
		}
	}
	if err := scheduler.prepareInputs(); err != nil {
		scheduler.logger.close()
		return Report{}, err
	}
	scheduler.logger.emit("run_started", "", 0, epoch, Running, "")
	report, runErr := scheduler.execute(ctx, started)
	if scheduler.logger != nil {
		scheduler.logger.emit("run_finished", "", 0, scheduler.epoch, report.Status, "")
		if err := scheduler.logger.close(); runErr == nil {
			runErr = err
		}
	}
	if err := writeReport(runDir, report, options.IncludeTasks); runErr == nil {
		runErr = err
	}
	return report, runErr
}

func (scheduler *scheduler) execute(parent context.Context, started time.Time) (Report, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	ticker := time.NewTicker(scheduler.options.PollInterval)
	defer ticker.Stop()

	for scheduler.finished < len(scheduler.tasks) {
		if !scheduler.stopped.Load() && !scheduler.options.InMemory && StopRequested(scheduler.options.StateDir) {
			scheduler.latchStop(cancel)
		}
		for !scheduler.stopped.Load() && scheduler.running < scheduler.workers && len(scheduler.ready) > 0 {
			id := <-scheduler.ready
			state := scheduler.tasks[id]
			if state.status != Pending {
				continue
			}
			state.status = Running
			state.result.Status = Running
			state.result.StartedAt = time.Now().UTC()
			scheduler.running++
			scheduler.logger.emit("task_started", id, 0, scheduler.epoch, Running, "")
			go scheduler.executeTask(ctx, state.task)
		}
		if scheduler.finished == len(scheduler.tasks) {
			break
		}
		select {
		case execution := <-scheduler.done:
			scheduler.running--
			scheduler.complete(execution)
		case <-ticker.C:
			if !scheduler.stopped.Load() && !scheduler.options.InMemory && StopRequested(scheduler.options.StateDir) {
				scheduler.latchStop(cancel)
			}
		case <-parent.Done():
			if !scheduler.stopped.Load() {
				if !scheduler.options.InMemory {
					_ = Stop(scheduler.options.StateDir, parent.Err().Error())
				}
				scheduler.latchStop(cancel)
			}
		}
	}
	return scheduler.report(started), nil
}

func (scheduler *scheduler) latchStop(cancel context.CancelFunc) {
	if !scheduler.stopped.CompareAndSwap(false, true) {
		return
	}
	fencedEpoch := scheduler.epoch + 1
	if record, err := readStop(scheduler.options.StateDir); err == nil && record.Epoch > scheduler.epoch {
		fencedEpoch = record.Epoch
	}
	cancel()
	for id, state := range scheduler.tasks {
		if state.status == Pending {
			scheduler.finish(state, Stopped, ErrStopped)
			scheduler.logger.emit("task_stopped", id, 0, fencedEpoch, Stopped, "epoch fenced")
		}
	}
	scheduler.logger.emit("swarm_stopped", "", 0, fencedEpoch, Stopped, "new work fenced")
}

func (scheduler *scheduler) executeTask(ctx context.Context, task Task) {
	block := scheduler.manifest.Blocks[task.Block]
	passes := PassCount(task, block)
	stdin := scheduler.initialInput(task)
	var lastLease Lease
	completed := 0
	for pass := 1; pass <= passes; pass++ {
		if ctx.Err() != nil || scheduler.stopped.Load() || (!scheduler.options.InMemory && StopRequested(scheduler.options.StateDir)) {
			scheduler.done <- taskExecution{id: task.ID, lease: lastLease, passes: completed, err: ErrStopped}
			return
		}
		lease := Lease{RunID: scheduler.runID, Epoch: scheduler.epoch, Sequence: scheduler.sequence.Add(1), TaskID: task.ID, Pass: pass}
		if !LeaseValid(lease, scheduler.epoch) {
			scheduler.done <- taskExecution{id: task.ID, lease: lease, passes: completed, err: ErrStopped}
			return
		}
		lastLease = lease
		if !scheduler.options.InMemory {
			if _, err := WriteLease(scheduler.runDir, lease); err != nil {
				scheduler.done <- taskExecution{id: task.ID, lease: lease, passes: completed, err: err}
				return
			}
		}
		output := passOutputPath(scheduler.runDir, task.ID, pass)
		scheduler.logger.emit("pass_started", task.ID, pass, lease.Epoch, Running, "")
		err := scheduler.executor.Execute(ctx, lease, task, block, stdin, output)
		if err != nil {
			scheduler.logger.emit("pass_failed", task.ID, pass, lease.Epoch, Failed, err.Error())
			scheduler.done <- taskExecution{id: task.ID, lease: lease, passes: completed, err: err}
			return
		}
		completed = pass
		scheduler.logger.emit("pass_finished", task.ID, pass, lease.Epoch, Succeeded, "")
		stdin = output
	}
	finalOutput := stdin
	if !scheduler.options.InMemory {
		if scheduler.stopped.Load() || StopRequested(scheduler.options.StateDir) {
			scheduler.done <- taskExecution{id: task.ID, lease: lastLease, passes: completed, err: ErrStopped}
			return
		}
		finalOutput = OutputPath(scheduler.runDir, task.ID)
		if err := os.Rename(stdin, finalOutput); err != nil {
			scheduler.done <- taskExecution{id: task.ID, lease: lastLease, passes: completed, err: err}
			return
		}
	}
	scheduler.done <- taskExecution{id: task.ID, lease: lastLease, passes: completed, output: finalOutput}
}

func (scheduler *scheduler) complete(execution taskExecution) {
	state := scheduler.tasks[execution.id]
	if scheduler.stopped.Load() || !LeaseValid(execution.lease, scheduler.epoch) || errors.Is(execution.err, ErrStopped) || errors.Is(execution.err, context.Canceled) {
		scheduler.finish(state, Stopped, ErrStopped)
		scheduler.logger.emit("completion_fenced", execution.id, execution.lease.Pass, scheduler.epoch, Stopped, "stale completion rejected")
		return
	}
	if execution.err != nil {
		scheduler.finish(state, Failed, execution.err)
		scheduler.logger.emit("task_failed", execution.id, execution.lease.Pass, scheduler.epoch, Failed, execution.err.Error())
		scheduler.skipDependents(execution.id)
		return
	}
	state.result.OutputPath = execution.output
	state.result.Passes = execution.passes
	scheduler.finish(state, Succeeded, nil)
	scheduler.logger.emit("task_finished", execution.id, execution.lease.Pass, scheduler.epoch, Succeeded, "")
	for _, child := range scheduler.dependents[execution.id] {
		state := scheduler.tasks[child]
		if state.status != Pending {
			continue
		}
		state.remaining--
		if state.remaining == 0 {
			scheduler.ready <- child
		}
	}
}

func (scheduler *scheduler) skipDependents(id string) {
	queue := append([]string(nil), scheduler.dependents[id]...)
	for len(queue) > 0 {
		child := queue[0]
		queue = queue[1:]
		state := scheduler.tasks[child]
		if state.status != Pending {
			continue
		}
		scheduler.finish(state, Skipped, errors.New("upstream task failed"))
		scheduler.logger.emit("task_skipped", child, 0, scheduler.epoch, Skipped, "upstream task failed")
		queue = append(queue, scheduler.dependents[child]...)
	}
}

func (scheduler *scheduler) finish(state *taskState, status Status, err error) {
	state.status = status
	state.result.Status = status
	state.result.FinishedAt = time.Now().UTC()
	if !state.result.StartedAt.IsZero() {
		state.result.Duration = state.result.FinishedAt.Sub(state.result.StartedAt)
	}
	if err != nil && status != Skipped {
		state.result.Error = err.Error()
	}
	scheduler.finished++
}

func (scheduler *scheduler) report(started time.Time) Report {
	finished := time.Now().UTC()
	report := Report{Version: 1, StartedAt: started, FinishedAt: finished, DurationMS: finished.Sub(started).Milliseconds(), Total: len(scheduler.tasks), Tasks: make(map[string]TaskResult, len(scheduler.tasks))}
	for id, state := range scheduler.tasks {
		report.Tasks[id] = state.result
		switch state.status {
		case Succeeded:
			report.Succeeded++
		case Failed:
			report.Failed++
		case Skipped:
			report.Skipped++
		case Stopped:
			report.Stopped++
		}
	}
	switch {
	case scheduler.stopped.Load():
		report.Status = Stopped
	case report.Failed > 0:
		report.Status = Failed
	default:
		report.Status = Succeeded
	}
	return report
}

func (scheduler *scheduler) prepareInputs() error {
	if scheduler.options.InMemory {
		return nil
	}
	for _, task := range scheduler.manifest.Tasks {
		if task.Input == "" {
			continue
		}
		if err := os.WriteFile(inputPath(scheduler.runDir, task.ID), []byte(task.Input), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (scheduler *scheduler) initialInput(task Task) string {
	switch {
	case task.Input != "":
		return inputPath(scheduler.runDir, task.ID)
	case task.InputFile != "":
		return task.InputFile
	case task.StdinFrom != "":
		return OutputPath(scheduler.runDir, task.StdinFrom)
	default:
		return ""
	}
}

func newRunID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func acquireRunLock(options Options, runID string, epoch uint64) (*os.File, error) {
	if options.InMemory {
		return nil, nil
	}
	path := filepath.Join(options.StateDir, LockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errors.New("another run holds this state directory")
		}
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%s\n%d\n%d\n", runID, epoch, os.Getpid()); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}
	return file, nil
}

func writeReport(runDir string, report Report, includeTasks bool) error {
	copy := report
	if !includeTasks {
		copy.Tasks = nil
	}
	data, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(runDir, "report.json"), data, 0o600)
}
