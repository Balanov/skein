package engine

import (
	"context"
	"time"
)

// Manifest is the stable, JSON-only swarm format. Commands are argv arrays:
// no shell is inserted between Skein and a workload.
type Manifest struct {
	Version int              `json:"version"`
	Workers int              `json:"workers,omitempty"`
	Limits  Limits           `json:"limits,omitempty"`
	Blocks  map[string]Block `json:"blocks"`
	Tasks   []Task           `json:"tasks"`
}

type Limits struct {
	MaxTasks  int `json:"max_tasks,omitempty"`
	MaxPasses int `json:"max_passes,omitempty"`
}

// Lease is a fencing token, not an authentication credential. A stop advances
// the swarm epoch; work and completions from older epochs are never admitted.
type Lease struct {
	RunID    string `json:"run_id"`
	Epoch    uint64 `json:"epoch"`
	Sequence uint64 `json:"sequence"`
	TaskID   string `json:"task_id"`
	Pass     int    `json:"pass"`
}

type Block struct {
	Argv      []string `json:"argv"`
	Dir       string   `json:"dir,omitempty"`
	Env       []string `json:"env,omitempty"`
	Passes    int      `json:"passes,omitempty"`
	TimeoutMS int64    `json:"timeout_ms,omitempty"`
}

type Task struct {
	ID        string   `json:"id"`
	Block     string   `json:"block"`
	Needs     []string `json:"needs,omitempty"`
	Input     string   `json:"input,omitempty"`
	InputFile string   `json:"input_file,omitempty"`
	StdinFrom string   `json:"stdin_from,omitempty"`
	Passes    int      `json:"passes,omitempty"`
}

type Status string

const (
	Pending   Status = "pending"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Skipped   Status = "skipped"
	Stopped   Status = "stopped"
)

type TaskResult struct {
	ID         string        `json:"id"`
	Status     Status        `json:"status"`
	Passes     int           `json:"passes_completed"`
	OutputPath string        `json:"output_path,omitempty"`
	Error      string        `json:"error,omitempty"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	FinishedAt time.Time     `json:"finished_at,omitempty"`
	Duration   time.Duration `json:"-"`
}

type Report struct {
	Version    int                   `json:"version"`
	Status     Status                `json:"status"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	DurationMS int64                 `json:"duration_ms"`
	Total      int                   `json:"total"`
	Succeeded  int                   `json:"succeeded"`
	Failed     int                   `json:"failed"`
	Skipped    int                   `json:"skipped"`
	Stopped    int                   `json:"stopped"`
	Tasks      map[string]TaskResult `json:"tasks,omitempty"`
}

type Event struct {
	Sequence uint64    `json:"seq"`
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Epoch    uint64    `json:"epoch"`
	TaskID   string    `json:"task,omitempty"`
	Pass     int       `json:"pass,omitempty"`
	Status   Status    `json:"status,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Executor lets benchmarks exercise the scheduler without starting processes.
// Real runs use ProcessExecutor; outputs always live outside the control log.
type Executor interface {
	Execute(ctx context.Context, lease Lease, task Task, block Block, stdinPath, stdoutPath string) error
}

type ExecutorFunc func(ctx context.Context, lease Lease, task Task, block Block, stdinPath, stdoutPath string) error

func (f ExecutorFunc) Execute(ctx context.Context, lease Lease, task Task, block Block, stdinPath, stdoutPath string) error {
	return f(ctx, lease, task, block, stdinPath, stdoutPath)
}
