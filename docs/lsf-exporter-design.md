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
- `/all-jobs` keeps an independent all-job cache and calls `ALL_JOB` only when `refresh=true` or `trigger=true` is passed.
- Collection duration, errors, skipped collections, and snapshot age are exported for alerting.

## Data Exposure

Prometheus receives stable numeric metrics and bounded labels. The full per-job payload is exposed through `/jobs` because exporting every job field as Prometheus labels would create high cardinality and can overload Prometheus.

Submitted CPU/slot count is copied from the LSF job submit record into `requested_cpu` and exported as `lsf_job_requested_cpu`. Used CPU is the LSF accumulated CPU time field and is exposed as `cpu_time_seconds` in JSON and `lsf_job_cpu_time_seconds` in Prometheus. Submitted memory is parsed from `rusage[mem=...]` into `requested_memory_kb` and `lsf_job_requested_memory_kilobytes`; used memory remains the LSF runtime usage field exposed as `memory_kb` and `lsf_job_memory_kilobytes`.

`/all-jobs` is intentionally separate from `/jobs`. It is not part of the Prometheus scrape path, and a read without `refresh=true` returns only the last independent all-job cache. A refresh performs one native `ALL_JOB` query, replaces only the all-job cache, and leaves the normal background snapshot untouched.

## Unsupported Data and Extension Boundaries

The current implementation intentionally exposes only job information returned by `lsb_openjobinfo` and `lsb_readjobinfo`. It does not query other LSF object families such as queues, hosts, cluster state, license usage, or scheduler diagnostics.

Unsupported data is grouped below to make future extensions explicit:

| Area | Current status | Reason | Possible extension path |
| --- | --- | --- | --- |
| Queue configuration and queue load | Supported through external JSON | The native C collector still only opens job queries. | Use `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` now; replace with a native queue source later if desired. |
| Host status, slots, and load | Supported through external JSON | Host data changes independently from job data. | Use an external command/script now; keep host metrics separate from job labels. |
| Cluster or master state | Supported through external JSON | `/healthz` still reflects exporter process health only. | Publish cluster/master state through external JSON or a future native source. |
| License usage | Supported through external JSON | License data is not part of `jobInfoEnt`. | Publish license feature totals through external JSON or a future dedicated collector. |
| Pending reason | Not collected | The current job field copy does not include scheduler pending reason details. | Extend job field extraction or add a diagnostic query; prefer `/jobs` JSON for verbose reason text. |
| Exit reason | Partially collected | The collector now copies `exitStatus`, but detailed termination diagnostics are still not mapped. | Keep `exit_status` as a numeric metric and add detailed reason text to JSON only when a stable LSF field is confirmed. |
| Job dependency | Partially collected | The submit dependency condition is copied to `/jobs`, but dependency resolution diagnostics are not collected. | Keep dependency expressions in JSON and expose only bounded aggregate metrics if needed. |
| Resource requirement details | Partially collected | The submit resource requirement string is copied to `/jobs`; it is intentionally excluded from Prometheus labels. | Parse selected low-cardinality dimensions later if operators define an allowlist. |
| GPU usage | Supported through external JSON | GPU usage is environment-specific and not mapped by the native job collector. | Publish `gpu_requested` and `gpu_used` through external JSON, or parse selected resource fields later. |
| Custom resource usage | Supported through external JSON | Custom resources vary by cluster and can create unbounded dimensions. | Publish allowlisted resources through external JSON. |

Future extensions should preserve the existing protection model: no LSF API calls in the scrape path, bounded Prometheus labels, separate collection intervals for expensive object families, and JSON endpoints for verbose or high-cardinality fields.

## External Resource Extension

The native cgo source remains responsible for job collection through the LSF C API. Queue, host, cluster, license, GPU, and site-specific custom resource data can be supplied through `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND`. The command must emit JSON matching the exporter `Data` schema: `jobs`, `queues`, `hosts`, `cluster`, `licenses`, and `custom_resources`.

The bundled `scripts/collect-extra.sh` keeps verbose queue relationships in JSON-only `raw` fields. Queue host relationships use `raw.host_spec`, `raw.host_spec_tokens`, `raw.host_groups`, and `raw.hosts`. Queue user relationships use `raw.user_spec`, `raw.user_spec_tokens`, `raw.user_groups`, and `raw.users`; `USERS: all` is expanded from `busers -w` when available.

This keeps the scrape path safe while allowing operators to integrate LSF CLI scripts, site-specific APIs, or future native C API collectors without changing the HTTP and Prometheus surface. Verbose or high-cardinality fields such as pending reasons, exit reasons, resource requirement strings, dependency expressions, command paths, and custom resource details remain JSON-first and should only become metrics after explicit low-cardinality parsing or allowlisting.
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
