#!/usr/bin/env python3
"""Measure no-op DAG execution without memory instrumentation in the timed region."""
import argparse
import asyncio
import importlib.metadata
import json
import platform
import time


async def noop():
    return None


async def after(previous):
    if previous is not None:
        await previous
    await noop()


async def signal_after(previous, done):
    if previous is not None:
        await previous.wait()
    await noop()
    done.set()


async def asyncio_graph(tasks, shape):
    if shape == "independent":
        await asyncio.gather(*(noop() for _ in range(tasks)))
        return
    if shape == "chain":
        events = [asyncio.Event() for _ in range(tasks)]
        nodes = [signal_after(events[index - 1] if index else None, events[index]) for index in range(tasks)]
        await asyncio.gather(*nodes)
        return
    source = asyncio.create_task(noop())
    leaves = [after(source) for _ in range(tasks - 2)]
    await asyncio.gather(*leaves)
    await noop()


def build_langgraph(tasks, shape):
    from langgraph.graph import END, StateGraph, START
    from typing_extensions import TypedDict

    class State(TypedDict, total=False):
        value: int

    graph = StateGraph(State)
    names = [f"task_{index:06d}" for index in range(tasks)]
    for name in names:
        graph.add_node(name, lambda _state: {})
    if shape == "independent":
        for name in names:
            graph.add_edge(START, name)
            graph.add_edge(name, END)
    elif shape == "chain":
        graph.add_edge(START, names[0])
        for previous, current in zip(names, names[1:]):
            graph.add_edge(previous, current)
        graph.add_edge(names[-1], END)
    else:
        graph.add_edge(START, names[0])
        for name in names[1:-1]:
            graph.add_edge(names[0], name)
            graph.add_edge(name, names[-1])
        graph.add_edge(names[-1], END)
    return graph.compile()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", choices=["asyncio", "langgraph"], required=True)
    parser.add_argument("--tasks", type=int, required=True)
    parser.add_argument("--shape", choices=["independent", "chain", "fanout"], default="independent")
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--warmups", type=int, default=2)
    args = parser.parse_args()
    if args.tasks < 1 or (args.shape == "fanout" and args.tasks < 3):
        parser.error("invalid task count for graph shape")

    compiled = build_langgraph(args.tasks, args.shape) if args.candidate == "langgraph" else None
    version = importlib.metadata.version("langgraph") if args.candidate == "langgraph" else platform.python_version()

    def invoke():
        if args.candidate == "langgraph":
            compiled.invoke({"value": 0})
        else:
            asyncio.run(asyncio_graph(args.tasks, args.shape))

    for _ in range(args.warmups):
        invoke()
    samples = []
    for run in range(args.runs):
        started = time.perf_counter_ns()
        invoke()
        duration = time.perf_counter_ns() - started
        samples.append({
            "run": run + 1,
            "duration_ns": duration,
            "tasks_per_second": args.tasks / (duration / 1e9),
            "succeeded": args.tasks,
        })
    print(json.dumps({
        "candidate": args.candidate,
        "candidate_version": version,
        "tasks": args.tasks,
        "shape": args.shape,
        "python": platform.python_version(),
        "implementation": platform.python_implementation(),
        "samples": samples,
    }))


if __name__ == "__main__":
    main()
