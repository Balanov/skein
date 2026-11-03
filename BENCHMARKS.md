# Reproducible measurements

These are local observations, not universal rankings. Warm scheduler overhead, cold process dispatch and emergency stop are separate questions; their numbers must not be mixed. Raw samples and reproduction scripts are included so another result can refute these numbers.

## Environment and statistical method

Recorded on 2026-11-03 local time:

- Windows 11 build 28020, AMD64; AMD Ryzen 9 5950X, 32 logical CPUs.
- Skein 0.1.0, Go 1.27.3; binaries built with `-trimpath -ldflags="-s -w"`.
- Python 3.12.16; LangGraph 1.2.19 with the exact transitive versions in [requirements-langgraph.txt](bench/requirements-langgraph.txt).
- Warm caches, no intentional concurrent workload.

For warm scheduler measurements, each candidate runs three batches. Candidate order rotates between batches; every batch has two warmups followed by five measured runs, producing 15 samples per candidate. Startup and LangGraph compilation are outside these samples. Skein constructs its manifest once per batch and validates it on every sample. No `tracemalloc` or other memory profiler runs inside the timed region.

All p95 values below use nearest rank. Go allocation counters and Python memory metrics are deliberately not used for a cross-runtime memory ranking.

During development a bare-goroutine lower bound was also measured to sanity-check scheduler overhead. It performs no manifest validation, persistence, reporting, audit events or fencing, so it is not a product comparison and is omitted from the tables below; its raw numbers remain in the linked sample files.

## Warm no-op DAGs at 1,000 nodes

Every candidate executes the same graph shape with empty node functions. There are no model calls, providers, network requests, checkpoints or workload processes. Each cell is **median / p95 wall time**.

| Graph shape | Skein | Python asyncio | LangGraph |
| --- | ---: | ---: | ---: |
| Independent | **1.580 / 2.377 ms** | 3.530 / 15.336 ms | 876.910 / 950.755 ms |
| Serial chain | **3.272 / 3.621 ms** | 4.479 / 15.308 ms | 534.907 / 538.510 ms |
| Fan-out / join | **2.080 / 2.409 ms** | 3.795 / 4.036 ms | 892.522 / 987.461 ms |

For these specific 1,000-node samples, Skein's median was:

- **2.23× lower than asyncio** and **554.9× lower than LangGraph** for independent tasks;
- **1.37× lower than asyncio** and **163.5× lower than LangGraph** for the serial chain;
- **1.82× lower than asyncio** and **429.0× lower than LangGraph** for fan-out / join.

LangGraph provides a broader graph/state feature set; these tables measure local graph scheduling only.

Raw data: [independent](bench/results/windows-amd64-scheduler-independent.json), [chain](bench/results/windows-amd64-scheduler-chain.json), [fan-out / join](bench/results/windows-amd64-scheduler-fanout.json).

## Small graphs: Skein is not always ahead

At 100 nodes, asyncio had the lower median in all three recorded shapes. Each cell is **median / p95 wall time**.

| Graph shape | Skein | Python asyncio | LangGraph |
| --- | ---: | ---: | ---: |
| Independent | 1.040 / 1.569 ms | **0.740 / 0.789 ms** | 22.278 / 48.163 ms |
| Serial chain | 1.042 / 1.544 ms | **0.855 / 1.103 ms** | 17.759 / 19.635 ms |
| Fan-out / join | 1.049 / 1.297 ms | **0.788 / 0.827 ms** | 22.170 / 49.959 ms |

For 100 independent tasks, asyncio's median was **1.40× lower than Skein's**. This counterexample is shown on the landing page rather than selecting only favorable sizes.

## Scale within the same process

LangGraph was not run above 1,000 nodes; no extrapolation is presented. Each cell is **median / p95 wall time**.

| Graph shape | Nodes | Skein | Python asyncio |
| --- | ---: | ---: | ---: |
| Independent | 10,000 | **11.589 / 18.045 ms** | 37.767 / 42.893 ms |
| Independent | 100,000 | **114.374 / 129.138 ms** | 505.848 / 542.264 ms |
| Serial chain | 10,000 | **29.975 / 32.935 ms** | 51.113 / 58.462 ms |
| Fan-out / join | 10,000 | **17.019 / 18.490 ms** | 40.203 / 51.316 ms |

At 100,000 independent no-op tasks, Skein's median was **4.42× lower than asyncio**. This demonstrates local scheduler throughput for the tested graph, not 100,000 concurrent model calls or OS processes. Real workloads are normally limited by providers, memory, tools and operating-system process limits.

## Cold runs with real workload processes

Every timed run starts a fresh candidate process. Candidate startup, imports, Skein manifest validation, LangGraph graph compilation, orchestration and workload-process startup are all inside the measured interval. Each of 24 children reads the same 16 KiB payload from stdin, computes SHA-256 and returns it. The concurrency limit is eight workers.

Candidate order rotates. There are three batches with one warmup and five measured fresh processes per candidate per batch: 15 samples and 360 verified results per candidate. Parent-side verification is outside the timed interval.

| Candidate | Median | p95 | Verified results |
| --- | ---: | ---: | ---: |
| Skein | **156.463 ms** | 171.590 ms | 360 / 360 |
| Python asyncio | 288.644 ms | 300.693 ms | 360 / 360 |
| LangGraph | 998.010 ms | 1,038.735 ms | 360 / 360 |

Skein's cold-process median was **1.84× lower than asyncio** and **6.38× lower than LangGraph** for this workload. These ratios do not belong to the warm scheduler tables and do not measure model quality or provider latency.

Raw data: [windows-amd64-process-workload.json](bench/results/windows-amd64-process-workload.json).

## Emergency stop with real parent/child trees

Three unrecorded warmups precede 20 measured runs. Each run starts eight Python parents and eight Python children, then waits until every parent and child is ready. The measured interval starts at the stop CLI launch and ends when the Skein run exits after terminating the tracked trees and writing its report.

Every child updates an atomic heartbeat file. After the run exits, all eight heartbeats must remain unchanged for 150 ms. These checks happen after the measured interval. Every run also verifies that a duplicate stop preserves the first epoch and that reset does not reuse an old epoch.

| Measured runs | Parent/child trees stopped | Child heartbeats frozen | Median | p95 | Max |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 20 | **160 / 160** | **160 / 160** | **141.663 ms** | **150.564 ms** | 163.894 ms |

This is an end-to-end local observation, not a guaranteed distributed-system shutdown deadline. Skein cannot forcibly stop remote work, detached processes or side effects that already crossed an external boundary. Epoch fencing prevents stale queued work and late completions from being accepted by this scheduler.

Raw data: [windows-amd64-stop.json](bench/results/windows-amd64-stop.json).

## Functional and adversarial checks

The recorded Go test run contains **147 passed test/subtest records**, including 15 top-level tests in two packages. It includes:

- **128 deterministic random DAGs containing 6,166 tasks**, checking dependency order and worker bounds;
- **800 concurrent stop calls across 25 rounds**, checking that the first latch remains unchanged and the epoch advances once;
- an executor that deliberately returns success after cancellation, checking that its late result and dependent are stopped;
- real parent/child termination with a heartbeat check.

The real CLI proof additionally passes **3 / 3 scenarios**: multi-pass data flow (`start:a/1:a/2:b/1`), dependency failure isolation and cycle rejection before execution.

Raw data: [Go test JSONL](bench/results/windows-amd64-tests.jsonl), [functional reports and events](bench/results/windows-amd64-functional.json).

## Reproduce

See [bench/README.md](bench/README.md) for exact commands and landing-data generation. CI runs the real CLI proof and parent/child stop proof on six native runners (Linux, Windows, macOS × amd64, arm64); cross-compilation alone is not presented as runtime testing.
