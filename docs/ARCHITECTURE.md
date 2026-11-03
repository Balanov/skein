# Architecture

Skein is a fail-closed swarm control plane. It is not an agent, model router, prompt framework, semantic tool-call verifier, sandbox, or durable multi-region workflow service. The core executes explicit manifests and makes one strong distinction: ordinary scheduling may be fast, but stopping must also invalidate work that is still queued or finishes too late.

## Epoch fence

Each run reads a persistent monotonically increasing epoch from the shared control directory. Every pass receives a lease containing the run ID, epoch, unique sequence, task ID, and pass number. A lease is a fencing token, not an identity credential or proof that an action is safe.

`skein stop` performs two coordinated effects:

1. It advances and persists the epoch, leaving an idempotent `STOP` latch.
2. The active scheduler observes the latch, cancels its context, terminates tracked process trees, and marks queued work stopped.

A reset removes the latch but never decrements or reuses the epoch. A crashed stop whose latch advanced farther than the epoch file is repaired during reset. New runs cannot start while the latch exists.

## Late-completion rule

Successful process exit is not sufficient to accept a result. Before publishing a final output, Skein checks the stop state again. When a completion reaches the scheduler, the scheduler rejects it if the run is stopped, the lease belongs to another epoch, or execution ended through cancellation. Rejected output cannot release dependent work. This closes the common race in which cancellation is requested, an old task exits successfully anyway, and its dependents start as though nothing happened.

The lease file and environment variables make the fencing context explicit to a workload. They do not force an unrelated remote API to honor the epoch. Enforcement at an external boundary requires a gateway that understands the same lease; that is outside the local core's guarantee.

## Shared looped blocks

A block is a reusable execution definition: argv, working directory, environment additions, pass count, and timeout. Tasks reference the block instead of duplicating it. Multiple passes feed the previous pass's output into the next pass's stdin, conceptually `B(B(h))` while keeping one block definition.

This is an execution-model borrowing from looped computation, not a neural-network implementation and not a claim of model-quality improvement. Fixed pass limits and per-pass timeouts bound local execution.

## Control and payload streams

Small JSONL events contain sequence, time, event kind, epoch, task, pass, status, and errors. Large stdin/stdout/stderr payloads stay in per-run data files. The audit stream therefore does not replicate every agent output, and downstream tasks consume explicit file paths rather than accumulating all prior payloads in scheduler memory.

Event syncing is optional. Without `--sync`, the buffered event stream is an audit aid, not a crash-durable journal. Skein does not claim exactly-once external side effects or automatic crash recovery.

## Scheduler and limits

The scheduler maintains dependency counts, a bounded set of active workers, and terminal task states. Independent ready tasks run concurrently. Failed tasks skip their downstream descendants without blocking unrelated branches. `max_tasks`, `max_passes`, worker count, and per-pass timeouts constrain the declared blast radius.

Commands are argv arrays, not shell-interpolated strings. Skein does not sandbox those commands: they retain the operating-system user's permissions. Use an OS sandbox or disposable environment for untrusted workloads.

## Process-tree stop

On Unix, workloads start in a new process group and cancellation kills that group. On Windows, cancellation uses `taskkill /PID ... /T /F`, with a direct process kill fallback. The tests start a real parent and child and verify that the child's heartbeat stops.

This is stronger than marking a task canceled, but it is not a universal kill guarantee. Work that escaped the tracked tree, was detached by another service, or executed remotely is outside this local mechanism.
