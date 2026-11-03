# Skein

**Fail-closed orchestration for local agent swarms.**

Skein executes explicit task graphs with bounded concurrency, reusable multi-pass blocks, and a persistent **epoch fence**. An emergency stop invalidates queued work and rejects late completions from the old epoch instead of merely labeling a task canceled.

Skein is an independent open-source project by [Paritet](https://paritet.dev/). It is an execution control plane, not an agent framework or a model runtime. Version 0.1.0 is an early release: exercise the stop proof against your own workloads before relying on it operationally.

[Architecture](docs/ARCHITECTURE.md) · [Security boundary](SECURITY.md) · [Benchmarks](BENCHMARKS.md) · [Stop proof](bench/README.md) · [Landing source](site/)

## Why Skein

A successful process exit does not prove that its result is still safe to accept. Between cancellation and a worker's final exit, stale output can otherwise release dependent actions.

Skein gives every pass a lease with a run ID, epoch, unique sequence, task ID, and pass number. `skein stop` advances a persistent epoch and leaves an idempotent `STOP` latch. The scheduler then cancels execution and terminates tracked local process trees. A late result from an earlier epoch cannot be accepted or unlock dependents. `skein reset` clears the latch after inspection without reusing an old epoch.

- **A concrete stop boundary:** epoch fencing, queued-work invalidation, late-completion rejection, and local process-tree termination.
- **Explicit execution:** JSON manifests, dependency graphs, argv arrays without an implicit shell, bounded workers, tasks, passes, and per-pass timeouts.
- **Reusable looped blocks:** define a block once; feed each pass's stdout into the next pass's stdin.
- **Separate control and payload paths:** small JSONL events stay apart from large stdin/stdout/stderr files.
- **Portable core:** Go standard library only; no daemon, database, provider key, or runtime dependency in the orchestrator.

## Build and run

Go 1.26 or newer is required to build from source.

```sh
git clone https://github.com/Balanov/skein
cd skein
go build -trimpath -o bin/skein ./cmd/skein
./bin/skein --version
```

On Windows, use `./bin/skein.exe`. For a smaller release binary, add `-ldflags="-s -w"`. Prebuilt binaries and checksums are attached to tagged releases.

Run the included three-task example. Python is used only by this example workload, not by Skein itself:

```sh
./bin/skein run --state .skein --workers 2 --include-tasks examples/fanout.json
```

The command prints a JSON report. Run artifacts are stored under `.skein/runs/<run-id>/`: `report.json`, optional `events.jsonl`, `leases/`, and `data/`. Add `--sync` to sync each audit event to disk, or `--no-events` to disable the event stream. By default, detailed per-task results are omitted from the report; `--include-tasks` retains them.

### Manifest

```json
{
  "version": 1,
  "workers": 2,
  "limits": {"max_tasks": 8, "max_passes": 3},
  "blocks": {
    "audit": {
      "argv": ["python", "-c", "import sys; data=sys.stdin.read(); print(data.upper(), end='')"],
      "passes": 1,
      "timeout_ms": 5000
    }
  },
  "tasks": [
    {"id": "left", "block": "audit", "input": "left branch"},
    {"id": "right", "block": "audit", "input": "right branch"},
    {"id": "join", "block": "audit", "needs": ["left"], "stdin_from": "left"}
  ]
}
```

A task can select exactly one input source: inline `input`, `input_file`, or `stdin_from` from a declared dependency. A block can also set `dir` and additional `env` entries (`KEY=value`). Task-level `passes` overrides the block default. Zero worker/pass values select runtime defaults; explicit limits are recommended for production manifests.

## Emergency stop

Use the same state directory from another terminal:

```sh
./bin/skein stop --state .skein --reason "manual emergency stop"
./bin/skein status --state .skein
# {"stopped":true,"epoch":2}

# Inspect the report and running workload state first.
./bin/skein reset --state .skein
./bin/skein status --state .skein
# {"stopped":false,"epoch":2}
```

A stop uses exit code **4** for the interrupted run. Exit code **3** means at least one task failed, **1** an operational error, and **2** invalid CLI usage. A successful run returns **0**.

Reset refuses to clear a live run lock. `--force` is only for a stale lock after independently verifying that no run is active. Never reset a shared state directory while another operator may still be stopping it.

To reproduce a real parent-and-child process-tree stop:

```sh
python bench/stop_proof.py --skein ./bin/skein --runs 12 --tasks 8
```

## What the guarantee does—and does not—cover

**Covered:** scheduling within a single local Skein run; a persistent stop latch; epoch-fenced queued work; rejection of late completions; termination of tracked local process trees on Unix and Windows.

**Not covered:** sandboxing arbitrary commands; authentication or signed leases; semantic validation of agent output; exactly-once external side effects; automatic crash recovery; distributed consensus; remote work, detached processes, or side effects that already crossed an external boundary.

A lease is a fencing token, not a credential. External systems do not automatically honor it; enforcement at that boundary requires a separate gateway that checks the lease. Manifest commands retain the operating-system user's permissions. Use an OS sandbox or disposable environment for untrusted workloads.

See [SECURITY.md](SECURITY.md) before deployment and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the exact state transitions.

## Measurements

On the recorded Windows 11 / Ryzen 9 5950X environment, Skein scheduled **100,000 independent no-op tasks in 116.676 ms median** (**857,076 tasks/s**). For 1,000 independent no-op tasks, its median was **1.558 ms**, compared with **10.908 ms** for asyncio and **2,461.654 ms** for LangGraph. The stop proof stopped **8 real process trees in 134.185 ms median** over 12 runs.

These are local control-plane observations, not universal rankings, model-quality results, or shutdown-deadline guarantees. Candidate startup and LangGraph compilation are outside those samples; Skein validates its in-memory manifest every run. See [BENCHMARKS.md](BENCHMARKS.md) for the full scope, caveats, raw samples, and exact reproduction commands.

## Checks

```sh
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./... # requires CGO and a C compiler
```

CI checks formatting, tests, vet, the race detector on Linux, cross-platform compilation, and the frontend build. Cross-compilation does not replace running platform-specific stop tests on every target OS.

## Landing page

The static bilingual landing source is in `site/`; its intended public path is `https://paritet.dev/skein`. It uses Vite and self-hosted IBM Plex fonts.

```sh
cd site
npm ci
npm run dev -- --host 127.0.0.1 --port 7100
npm run build # static output in site/dist/
```

The epoch-stop visualization is a browser demonstration, not a remote control for a live swarm. See [CREDITS.md](CREDITS.md) for asset licenses.

## License and citation

MIT © 2026 Balanov. See [LICENSE](LICENSE). Machine-readable citation metadata is in [CITATION.cff](CITATION.cff).

```bibtex
@misc{balanov2026skein,
  title  = {Skein: Fail-Closed Swarm Orchestration},
  author = {Balanov},
  year   = {2026},
  url    = {https://paritet.dev/skein},
  note   = {MIT License}
}
```
