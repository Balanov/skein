# Reproduce the measurements and proofs

Build the real binaries first. Go 1.26+ is required; the recorded run used Go 1.27.3.

```sh
go build -trimpath -ldflags="-s -w" -o bin/skein ./cmd/skein
go build -trimpath -ldflags="-s -w" -o bin/skein-bench ./cmd/skein-bench
```

On Windows, use `bin/skein.exe` and `bin/skein-bench.exe`.

## Python candidates

The comparative runs need Python 3 and an isolated environment:

```sh
python -m venv bench/.venv
# Unix: bench/.venv/bin/python
# Windows: bench/.venv/Scripts/python.exe
bench/.venv/bin/python -m pip install -r bench/requirements-langgraph.txt
```

Use that interpreter for the commands below, replacing `bench/.venv/bin/python` with `bench/.venv/Scripts/python.exe` on Windows. Use `--python` if workload children should use another interpreter.

## Warm scheduler matrix

Each candidate runs three rotated batches with two warmups and five measured runs per batch. Startup and LangGraph compilation are excluded. No memory profiler runs in the timed region. Raw Go averages ten graph executions per sample; other candidates run one graph per sample.

```sh
bench/.venv/bin/python bench/compare.py \
  --skein-bench ./bin/skein-bench --shape independent \
  --tasks 100 1000 10000 100000 --runs 5 --batches 3 --warmups 2 \
  --output bench/results/local-scheduler-independent.json

bench/.venv/bin/python bench/compare.py \
  --skein-bench ./bin/skein-bench --shape chain \
  --tasks 100 1000 10000 --runs 5 --batches 3 --warmups 2 \
  --output bench/results/local-scheduler-chain.json

bench/.venv/bin/python bench/compare.py \
  --skein-bench ./bin/skein-bench --shape fanout \
  --tasks 100 1000 10000 --runs 5 --batches 3 --warmups 2 \
  --output bench/results/local-scheduler-fanout.json
```

LangGraph is skipped above `--langgraph-max 1000`; the raw JSON records the skip instead of extrapolating. The raw Go control has no validation, fencing, reports or audit events.

## Cold real-process comparison

Each timed run starts a fresh candidate and 24 real Python children with a concurrency limit of eight. Every child computes SHA-256 for the same 16 KiB stdin payload; every digest is verified. Startup, imports, manifest validation and LangGraph compilation are included.

```sh
bench/.venv/bin/python bench/workload_compare.py \
  --skein ./bin/skein --tasks 24 --workers 8 \
  --runs 5 --batches 3 --warmups 1 \
  --output bench/results/local-process-workload.json
```

## Real parent/child stop proof

```sh
bench/.venv/bin/python bench/stop_proof.py \
  --skein ./bin/skein --runs 20 --warmups 3 --tasks 8 \
  --output bench/results/local-stop.json
```

Every run starts eight parents and eight children. The script waits for all readiness markers, invokes the stop CLI, checks exit code 4 and the final report, and verifies that every child heartbeat remains unchanged for 150 ms. It also checks duplicate-stop idempotence and epoch-preserving reset. On Windows the run exercises `taskkill /T /F`; on Unix it exercises process-group termination.

## Real CLI functional proof

```sh
bench/.venv/bin/python bench/functional_proof.py \
  --skein ./bin/skein \
  --output bench/results/local-functional.json
```

This executes actual workload processes and checks multi-pass data flow, dependency failure isolation and cycle rejection. The JSON preserves observed output, statuses, diagnostics and the multi-pass event stream.

## Go tests and benchmarks

```sh
go test -count=1 ./...
go vet ./...
go test -count=1 -json ./... > bench/results/local-tests.jsonl
go test -run=NONE -bench=BenchmarkIndependentScheduler -benchmem ./internal/engine
```

The test suite includes 128 deterministic random DAGs and 800 concurrent stop calls. For a race-enabled run on a machine with CGO and a C compiler:

```sh
CGO_ENABLED=1 go test -race -count=1 ./...
```

## Generate the landing evidence

The landing does not maintain handwritten benchmark values. After recording files with the exact names in `bench/render_results.py`, run:

```sh
bench/.venv/bin/python bench/render_results.py
cd site
npm ci
npm run build
```

The script validates the expected input files, generates `site/src/results.js`, and copies the exact raw JSON/JSONL into `site/public/results/`. Vite includes them in the static build, so the published page can expose both compact charts and the underlying evidence without a backend.

CI runs the real CLI and stop proofs on Ubuntu, Windows and macOS and uploads their JSON artifacts. Cross-compilation alone is not treated as a platform runtime test.
