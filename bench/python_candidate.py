#!/usr/bin/env python3
"""Measure equivalent independent no-op graphs without model or process work."""
import argparse
import asyncio
import importlib.metadata
import json
import os
import platform
import time
import tracemalloc


async def noop():
    return None


async def asyncio_graph(tasks):
    await asyncio.gather(*(noop() for _ in range(tasks)))


def build_langgraph(tasks):
    from langgraph.graph import END, StateGraph, START
    from typing_extensions import TypedDict

    class State(TypedDict, total=False):
        value: int

    graph = StateGraph(State)
    for index in range(tasks):
        name = f"task_{index:06d}"
        graph.add_node(name, lambda _state: {})
        graph.add_edge(START, name)
        graph.add_edge(name, END)
    return graph.compile()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", choices=["asyncio", "langgraph"], required=True)
    parser.add_argument("--tasks", type=int, required=True)
    parser.add_argument("--runs", type=int, default=9)
    parser.add_argument("--warmups", type=int, default=2)
    args = parser.parse_args()

    compiled = build_langgraph(args.tasks) if args.candidate == "langgraph" else None
    version = importlib.metadata.version("langgraph") if args.candidate == "langgraph" else platform.python_version()

    def invoke():
        if args.candidate == "langgraph":
            compiled.invoke({"value": 0})
        else:
            asyncio.run(asyncio_graph(args.tasks))

    for _ in range(args.warmups):
        invoke()
    samples = []
    for run in range(args.runs):
        tracemalloc.start()
        started = time.perf_counter_ns()
        invoke()
        duration = time.perf_counter_ns() - started
        _, peak = tracemalloc.get_traced_memory()
        tracemalloc.stop()
        samples.append({
            "run": run + 1,
            "duration_ns": duration,
            "tasks_per_second": args.tasks / (duration / 1e9),
            "tracemalloc_peak_bytes": peak,
        })
    print(json.dumps({
        "candidate": args.candidate,
        "candidate_version": version,
        "tasks": args.tasks,
        "python": platform.python_version(),
        "implementation": platform.python_implementation(),
        "samples": samples,
    }))


if __name__ == "__main__":
    main()
