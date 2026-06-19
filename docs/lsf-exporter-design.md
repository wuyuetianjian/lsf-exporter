# LSF Exporter Design

## Goals

Build a Prometheus exporter that reads job information directly through IBM Spectrum LSF C APIs. The highest-priority constraint is that the exporter must not harm the LSF cluster, especially the LSF master.

## Architecture

The exporter keeps LSF API calls out of the Prometheus scrape path. A single background collector periodically calls the LSF C API, copies job fields into Go memory, closes LSF job query resources, and atomically publishes an in-memory snapshot. Prometheus scrapes only read that snapshot.

Production builds use `go build -tags lsf`, which enables the cgo implementation that calls `lsb_init`, `lsb_openjobinfo`, `lsb_readjobinfo`, and `lsb_closejobinfo`. Non-LSF development builds use a stub collector so non-cgo logic can be tested without IBM LSF SDK headers.

## Cluster Protection Rules

- Only one LSF collection can run at a time.
- A minimum collection interval is enforced even if Prometheus scrapes faster.
- If a collection is still running, the next scheduled run is skipped.
- LSF query results are copied into Go-owned memory before `lsb_closejobinfo`.
- Prometheus `/metrics` never calls LSF APIs.
- `/jobs` returns the cached full job snapshot as JSON.
- Collection duration, errors, skipped collections, and snapshot age are exported for alerting.

## Data Exposure

Prometheus receives stable numeric metrics and bounded labels. The full per-job payload is exposed through `/jobs` because exporting every job field as Prometheus labels would create high cardinality and can overload Prometheus.

## Operational Defaults

The default collection interval is 30 seconds. Operators can tune it, but the exporter rejects intervals below the configured minimum interval, which defaults to 10 seconds. The default HTTP listen address is `:9818`.

## Build

Production:

```sh
go build -tags lsf ./cmd/lsf-exporter
```

Development without LSF SDK:

```sh
go test ./...
```
