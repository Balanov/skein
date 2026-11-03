# Reproducible measurements

These are local observations, not universal rankings. They measure Skein's control plane, not model quality, token use, network latency, harness startup, or end-to-end task completion. Raw samples and reproduction scripts are included so another result can refute these numbers.

## Environment

Recorded on 2026-11-03 local time:

- Windows 11 build 28020, AMD64; AMD Ryzen 9 5950X, 32 logical CPUs.
- Skein 0.1.0, Go 1.27.3; binary built with `-trimpath -ldflags="-s -w"`.
- Python 3.12.16; LangGraph 1.2.19 with the exact transitive versions in [requirements-langgraph.txt](bench/requirements-langgraph.txt).
- Warm caches, no intentional concurrent workload. Candidate startup and LangGraph graph compilation are outside the measured samples. Skein validates its in-memory manifest on every sample.

The stripped Windows `skein.exe` is **3,342,848 bytes (3.19 MiB)**. It uses only the Go standard library.

## Independent no-op graph: framework comparison

Every candidate executes the same number of independent empty tasks. There are no model calls, providers, network requests, checkpoints, or workload processes. Nine measured runs follow two warmups. Values are medians of wall time; tasks/s is the median of per-run rates.

| Tasks | Candidate | Median | p95 | Median tasks/s |
| ---: | --- | ---: | ---: | ---: |
| 100 | Skein | **1.044 ms** | 1.646 ms | **95,822** |
| 100 | Python asyncio | 1.313 ms | 1.430 ms | 76,161 |
| 100 | LangGraph | 64.494 ms | 90.877 ms | 1,551 |
| 1,000 | Skein | **1.558 ms** | 2.283 ms | **641,972** |
| 1,000 | Python asyncio | 10.908 ms | 18.772 ms | 91,680 |
| 1,000 | LangGraph | 2,461.654 ms | 2,532.309 ms | 406 |

At 1,000 tasks, Skein's median was **7.0× lower than asyncio** and **about 1,580× lower than LangGraph** for this narrow workload. This does not imply a similar ratio for model-driven applications. LangGraph supplies a broader graph/state feature set; the benchmark deliberately isolates execution overhead rather than scoring those features.

Raw data: [windows-amd64-scheduler-compare.json](bench/results/windows-amd64-scheduler-compare.json).

## Scale within the same process

Seven measured runs follow two warmups. LangGraph was not run at these sizes; no extrapolation is presented for it.

| Tasks | Candidate | Median | p95 | Median tasks/s |
| ---: | --- | ---: | ---: | ---: |
| 10,000 | Skein | **12.180 ms** | 15.405 ms | **821,038** |
| 10,000 | Python asyncio | 135.364 ms | 142.757 ms | 73,875 |
| 100,000 | Skein | **116.676 ms** | 129.569 ms | **857,076** |
| 100,000 | Python asyncio | 1,570.802 ms | 1,596.073 ms | 63,662 |

At 100,000 independent no-op tasks, Skein's median was **13.5× lower than asyncio**. This demonstrates local scheduler throughput for the tested graph; it is not a claim that one machine can run 100,000 model calls or OS processes concurrently. Skein's worker limit remains in force, and real workloads are normally limited by providers, memory, tools, and operating-system process limits.

Raw data: [windows-amd64-scheduler-scale.json](bench/results/windows-amd64-scheduler-scale.json).

## Emergency stop with real process trees

Each of 12 runs starts eight Python workloads, waits until all eight readiness files exist, invokes `skein stop`, and measures until the run process exits. The checked result must have exit code 4, status `stopped`, zero succeeded tasks, and eight stopped tasks. The interval includes the stop CLI launch, latch propagation, Windows process-tree termination, and report output.

| Workloads per run | Median | p95 | Max |
| ---: | ---: | ---: | ---: |
| 8 | **134.185 ms** | **146.292 ms** | 146.292 ms |

This is an end-to-end local observation, not a guaranteed distributed-system shutdown deadline. Skein can terminate tracked local process trees; it cannot forcibly stop remote work, detached processes, or side effects that already crossed an external system boundary. Epoch fencing prevents stale queued work and late completions from being accepted by this scheduler.

Raw data: [windows-amd64-stop.json](bench/results/windows-amd64-stop.json).

## Reproduce

See [bench/README.md](bench/README.md) for exact commands. The test suite additionally checks graph validation, looped-block data flow, dependency skipping, worker limits, stale completion rejection, epoch persistence after interrupted stops, and real child-process termination.
