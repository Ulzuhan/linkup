# ADR 0001: Go for the redirect engine

- **Status:** Accepted
- **Date:** 2026-09-01

## Context

Every other service in this ecosystem is a Next.js application. Uniformity has
real value — one toolchain, one CI shape, one set of deployment habits — and
breaking it needs a better reason than novelty.

A URL shortener is not shaped like those services. Its hot path does almost
nothing: take a slug, resolve it, answer with a redirect. It has no rendering to
speak of, its dashboard is a handful of pages, and it is expected to stay
resident and idle for long stretches, on the same box as two dozen other
containers.

## Decision

The redirect engine is written in Go, with an in-memory LRU cache in front of
SQLite in WAL mode. The dashboard is server-rendered from templates embedded in
the binary, so the whole service is one static file.

## Why — including the argument that does not hold

**The weak argument, stated as weak: latency.** A warm resolution measures about
230 ns on the machine this runs on. A Node runtime would be somewhere around a
couple of milliseconds. Against a network round trip of 20–100 ms, *that
difference is noise*. Nobody has ever perceived it and nobody ever will. It is
the argument a benchmark makes look impressive and it is not why this decision
was taken.

**Footprint.** The container is capped at 128 MB and does not come close to it.
The Next.js services on the same host are capped at 1 GB. On a box running two
dozen containers, that ratio is the difference between adding a service and
choosing which one to remove.

**Attack surface.** A statically linked binary on a minimal base has no runtime
and no dependency tree to scan. The weekly vulnerability scan on a Node image
has thousands of packages to consider; here it has the base image and a handful
of Go modules. Less to patch is less to patch under pressure.

**Concurrency for short I/O.** The workload is many small, independent, mostly
cache-hit operations. Goroutines fit that shape without a thought.

## What this costs

A second toolchain, a second CI shape, and a second translation of every
deployment convention — all of which were written for Node services. That cost
is real and recurring, and it is paid on every change to the shared standard.

It is worth paying here because the service is small and its runtime profile is
genuinely different. It would not be worth paying for a service that looked like
the others.

## When this should be revisited

If the dashboard grows until it weighs more than the engine. At that point the
workload stops being "resolve and redirect" and starts being an application,
and the reasons above stop applying in the order they are written.

## Alternatives considered

- **Next.js, like the rest.** Cheapest in habit, worst in footprint. Rejected on
  memory ceiling, not on speed.
- **Rust.** Better still on both counts and considerably slower to write, with
  no second maintainer in sight. The gap over Go here is not where the cost is.
- **A hosted shortener.** Rejected outright: the product exists because hosted
  shorteners are the surveillance this one refuses to do.
