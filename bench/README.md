# Reproduce the measurements

Build the real binaries first. Go 1.26+ is required; the recorded run used Go 1.27.3.

```sh
go build -trimpath -ldflags="-s -w" -o bin/skein ./cmd/skein
go build -trimpath -ldflags="-s -w" -o bin/skein-bench ./cmd/skein-bench
```

On Windows, use `bin/skein.exe` and `bin/skein-bench.exe`.

## Scheduler comparison

The comparative run needs Python 3 and an isolated environment:

```sh
python -m venv bench/.venv
# Unix: bench/.venv/bin/python
# Windows: bench/.venv/Scripts/python.exe
bench/.venv/bin/python -m pip install -r bench/requirements-langgraph.txt
bench/.venv/bin/python bench/compare.py \
  --skein-bench ./bin/skein-bench \
  --tasks 100 1000 --runs 9 --warmups 2 \
  --output bench/results/local-scheduler-compare.json
```

Use the same interpreter for `--python` if it is not the one running the script. To reproduce the scale run without LangGraph, use a Python environment where LangGraph is absent:

```sh
python bench/compare.py \
  --skein-bench ./bin/skein-bench \
  --tasks 10000 100000 --runs 7 --warmups 2 \
  --output bench/results/local-scheduler-scale.json
```

The candidate scripts measure independent no-op tasks. `asyncio` creates and gathers native asyncio tasks. LangGraph compiles a graph of the same size once, then invokes it repeatedly with empty node functions. Skein executes an already constructed in-memory manifest but validates it on every run. Startup and compilation are not hidden inside the samples. Raw per-run durations and rates are preserved in the JSON output.

Go's allocation counters and Python's `tracemalloc` peaks measure different things and are deliberately not used for a cross-runtime memory ranking.

## Real stop proof

```sh
python bench/stop_proof.py \
  --skein ./bin/skein --runs 12 --tasks 8 \
  --output bench/results/local-stop.json
```

The script starts actual Python process trees, waits for every readiness marker, invokes the stop CLI, and checks both the exit code and the final report. On Windows it exercises `taskkill /T /F`; on Unix it exercises process-group termination.

## Go benchmark and checks

```sh
go test -count=1 ./...
go vet ./...
go test -run=NONE -bench=BenchmarkIndependentScheduler -benchmem ./internal/engine
```

For a race-enabled run on a machine with CGO and a C compiler:

```sh
CGO_ENABLED=1 go test -race -count=1 ./...
```
