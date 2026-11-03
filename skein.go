// Package skein is the public embedding API for Skein. It exposes the same
// execution core as the CLI: explicit manifests, bounded workers, epoch
// fencing, reports and the audit event stream. For non-Go hosts, embed the
// CLI binary instead — the manifest and report formats are identical.
package skein

import (
	"context"

	"github.com/Balanov/skein/internal/engine"
)

// Manifest, Block, Task, Limits, Lease, Report, TaskResult and Status are the
// stable JSON-facing types shared with the CLI manifest format.
type (
	Manifest   = engine.Manifest
	Block      = engine.Block
	Task       = engine.Task
	Limits     = engine.Limits
	Lease      = engine.Lease
	Report     = engine.Report
	TaskResult = engine.TaskResult
	Status     = engine.Status
)

const (
	Pending   = engine.Pending
	Running   = engine.Running
	Succeeded = engine.Succeeded
	Failed    = engine.Failed
	Skipped   = engine.Skipped
	Stopped   = engine.Stopped
)

// ErrStopped is returned when a run cannot start because the stop latch is set.
var ErrStopped = engine.ErrStopped

// Options controls a Run. StateDir is required for production use: it holds
// the epoch, the stop latch, run locks and the run artifacts. Events are
// always written to <state-dir>/runs/<run-id>/events.jsonl; SyncEvents makes
// every event durable before continuing.
type Options struct {
	StateDir     string
	Workers      int
	SyncEvents   bool
	IncludeTasks bool
}

// Run validates the manifest, executes the graph and returns the report.
// When a stop intervenes, running tasks are terminated and queued work is
// fenced; the report status is Stopped and the error is ErrStopped.
func Run(ctx context.Context, manifest Manifest, options Options) (Report, error) {
	return engine.Run(ctx, manifest, engine.Options{
		StateDir:     options.StateDir,
		Workers:      options.Workers,
		Events:       true,
		SyncEvents:   options.SyncEvents,
		IncludeTasks: options.IncludeTasks,
	})
}

// LoadManifest reads and validates a manifest file with unknown-field rejection.
func LoadManifest(path string) (Manifest, error) {
	return engine.LoadManifest(path)
}

// Validate checks a manifest without touching the filesystem.
func Validate(manifest Manifest) error {
	return engine.Validate(manifest)
}

// Stop persists the stop latch and advances the epoch. It is idempotent:
// the first stop wins and repeated calls change nothing.
func Stop(stateDir, reason string) error {
	return engine.Stop(stateDir, reason)
}

// Reset clears the stop latch after inspection. The epoch is never reused.
// Force is only for a stale run lock after verifying no run is active.
func Reset(stateDir string, force bool) error {
	return engine.Reset(stateDir, force)
}

// State reports whether the stop latch is set and the current epoch.
func State(stateDir string) (stopped bool, epoch uint64, err error) {
	epoch, err = engine.CurrentEpoch(stateDir)
	if err != nil {
		return false, 0, err
	}
	return engine.StopRequested(stateDir), epoch, nil
}
