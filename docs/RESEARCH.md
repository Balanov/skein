# Design context and project boundary

Skein was scoped around a narrow operational problem: a swarm needs a stop boundary that also governs queued work and late results. Another agent harness, generic DAG runner, or durable workflow service would not address that gap by itself.

The projects below were examined as design context. They are not dependencies of Skein, and no compatibility or integration is implied.

## AutoSwarm

[AutoSwarm](https://github.com/HKUDS/AutoSwarm) focuses on coordinating heterogeneous agents, their context, tools, and task delegation. Its model highlights the importance of contribution dependencies and explicit completion criteria.

Skein takes the narrower control-plane position: work is already described by an explicit graph, and the scheduler must be able to invalidate that graph safely. Skein does not select agents, share their memory, or choose tools.

## SenseNova Loop

[SenseNova-Loop](https://github.com/OpenSenseNova/SenseNova-Loop) explores repeated computation with shared model layers. Its execution shape motivated Skein's reusable multi-pass block: define execution once, then feed the previous pass's output to the next pass, conceptually `B(B(h))`.

The borrowing is limited to execution structure. Skein is not a neural-network implementation, does not share model weights, and makes no claim that repetition improves model quality.

## Restate

[Restate](https://github.com/restatedev/restate) provides durable execution, reliable communication, durable promises and timers, and stateful entities across failures and processes. That is a substantially broader persistence and recovery problem than Skein attempts to solve.

Skein therefore does not reimplement durable workflows. Version 0.1.0 explicitly excludes automatic crash recovery, distributed consensus, and exactly-once external side effects.

## Dagu and generic DAG runners

[Dagu](https://github.com/dagucloud/dagu) already offers local-first declarative workflows, dependencies, schedules, retries, logs, run history, a Web UI, and multiple execution topologies. Lightweight DAG runners also cover ordered command execution well.

A new DAG syntax alone would not be a meaningful product boundary. Skein's distinguishing mechanism is the persistent epoch fence and its acceptance rule for late completions, not the existence of a dependency graph.

## A2A CancelTask

The [A2A specification](https://a2a-protocol.org/latest/specification/) defines interoperability between independent agent systems. Its `CancelTask` operation explicitly attempts cancellation without guaranteeing success, because work may already have completed or may not be cancelable at its current stage.

Skein addresses a different layer: a local scheduler-wide epoch, queued-work fencing, and a rule preventing stale output from releasing dependent work. A2A cancellation is not a substitute for those local invariants. Skein 0.1.0 does not implement an A2A client, server, or adapter.

## Resulting product boundary

Skein combines a small local scheduler with a precise, testable stop mechanism:

1. Every dispatched pass carries an explicit epoch and unique sequence.
2. Stop advances a persistent epoch and leaves an idempotent latch.
3. Tracked local process trees are terminated.
4. Late completions cannot be accepted or release dependents.
5. Reset never reuses an old epoch.

This boundary keeps the core independently useful while leaving agent semantics, durable distributed execution, remote enforcement, and OS sandboxing to the systems designed for those responsibilities.
