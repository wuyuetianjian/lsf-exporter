# lsf-exporter

Prometheus exporter for IBM Spectrum LSF job information using the LSF C API.

The exporter is designed to protect the LSF master:

- LSF API calls run only in one background collection loop.
- Prometheus scrapes read an in-memory snapshot and never call LSF directly.
- Collections are single-flight; overlapping scans are skipped.
- A minimum collection interval is enforced.
- `lsb_closejobinfo()` is called on every `lsb_openjobinfo()` success path.
- Completed/finished jobs are not queried by default.

## Build

Install the IBM Spectrum LSF SDK and make sure headers and libraries are visible to cgo. Then build with the `lsf` tag:

```sh
go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter
```

If headers or libraries are not in default paths, set cgo flags:

```sh
CGO_CFLAGS="-I/path/to/lsf/include" CGO_LDFLAGS="-L/path/to/lsf/lib" \
  go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter
```

Development builds without `-tags lsf` use a stub collector and are only useful for tests.

## Run

```sh
./lsf-exporter
```

Systemd deployment files are provided under `deploy/`:

```sh
install -D -m 0755 lsf-exporter /opt/lsf-exporter/lsf-exporter
install -D -m 0755 scripts/collect-extra.sh /opt/lsf-exporter/collect-extra.sh
install -D -m 0644 deploy/lsf-exporter.env.example /etc/sysconfig/lsf-exporter
install -D -m 0644 deploy/lsf-exporter.service /etc/systemd/system/lsf-exporter.service
systemctl daemon-reload
systemctl enable --now lsf-exporter
```

Edit `/etc/sysconfig/lsf-exporter` before starting if your LSF paths differ from `/tools/lsf`.

Endpoints:

- `/metrics`: Prometheus metrics.
- `/jobs`: cached full snapshot as JSON, including copied job fields.
- `/all-jobs`: independent all-job query cache. Add `?refresh=true` or `?trigger=true` to run an `ALL_JOB` query and refresh this cache.
- `/finished-jobs`: finished job detail view filtered from the independent all-job cache. Add `?refresh=true` or `?trigger=true` to refresh this cache first.
- `/snapshot`: cached full snapshot as JSON.
- `/queues`, `/hosts`, `/cluster`, `/licenses`, `/resources`: resource-specific JSON views.
- `/healthz`: process health.

## Configuration

Environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `LSF_EXPORTER_LISTEN_ADDRESS` | `:9818` | HTTP listen address. |
| `LSF_EXPORTER_INTERVAL` | `30s` | Background collection interval. |
| `LSF_EXPORTER_MIN_INTERVAL` | `10s` | Minimum allowed interval between LSF API scans. |
| `LSF_EXPORTER_COLLECT_TIMEOUT` | `20s` | Reserved for operation alerting; LSF C calls cannot be safely interrupted by Go. |
| `LSF_EXPORTER_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `LSF_EXPORTER_APP_NAME` | `lsf-exporter` | LSF application name passed to `lsb_init`. |
| `LSF_EXPORTER_QUERY_USER` | `all` | User filter passed to `lsb_openjobinfo`. |
| `LSF_EXPORTER_QUERY_QUEUE` | empty | Queue filter. |
| `LSF_EXPORTER_QUERY_HOST` | empty | Host filter. |
| `LSF_EXPORTER_QUERY_JOB_NAME` | empty | Job name filter. |
| `LSF_EXPORTER_QUERY_JOB_ID` | `0` | Specific job ID; `0` means no job ID filter. |
| `LSF_EXPORTER_ALL_JOBS` | `false` | Use `ALL_JOB` instead of safer `CUR_JOB`. Enable only after capacity review. |
| `LSF_EXPORTER_DISABLE_NATIVE_COLLECTOR` | `false` | Disable the native LSF C API collector and rely on external JSON collection only. |
| `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` | empty | Optional command that writes JSON for queues, hosts, cluster, licenses, custom resources, or extra job details. |
| `LSF_EXPORTER_EXTERNAL_RESOURCE_TIMEOUT` | `10s` | Timeout for the external resource command. |

## All-Job Query Endpoint

`/jobs` remains the normal cached exporter snapshot and is refreshed only by the background collection loop. `/all-jobs` is separate and is intended for manual or controlled reads of historical jobs.

- `GET /all-jobs` returns the last independent all-job cache without calling LSF.
- `GET /all-jobs?refresh=true` runs one native `ALL_JOB` query and replaces the `/all-jobs` cache.
- `GET /all-jobs?trigger=true` is accepted as an alias for `refresh=true`.
- `GET /finished-jobs` returns only `DONE` and `EXIT` jobs from the same independent all-job cache.
- `GET /finished-jobs?refresh=true` refreshes the all-job cache first, then returns only finished job details.

The response is wrapped with `scope` and `refreshed` so callers can distinguish it from `/jobs`. `/all-jobs` uses `scope: "all_jobs"` and `/finished-jobs` uses `scope: "finished_jobs"`. Refreshes are single-flight and respect `LSF_EXPORTER_MIN_INTERVAL`; concurrent refreshes return `409`, and refreshes triggered too soon return `429`.

## Prometheus Metrics

Important self-monitoring metrics:

- `lsf_exporter_up`
- `lsf_exporter_collections_total`
- `lsf_exporter_collect_errors_total`
- `lsf_exporter_collect_skipped_total`
- `lsf_exporter_last_success_timestamp_seconds`
- `lsf_exporter_snapshot_queues`
- `lsf_exporter_snapshot_hosts`
- `lsf_exporter_snapshot_licenses`
- `lsf_exporter_snapshot_custom_resources`
- `lsf_exporter_snapshot_age_seconds`
- `lsf_exporter_last_collect_duration_seconds`

Job metrics:

The `/jobs` JSON payload also includes job detail fields that are intentionally not exposed as Prometheus labels, including `exit_status`, `requested_cpu`, `requested_memory_kb`, `resource_requirement`, and `dependency_condition`. `requested_cpu` is the submitted CPU/slot count from LSF, while `cpu_time_seconds` is the accumulated CPU time used by the job. `requested_memory_kb` is parsed from `rusage[mem=...]`, while `memory_kb` is converted from the LSF `jobInfoEnt.maxMem` value so it matches `bjobs` `max_mem`/Max Memory output.

When LSF returns the same execution host multiple times for multi-slot jobs, `execution_host` is compacted as `N * host`, for example `4 * host-a`.

- `lsf_job_info`
- `lsf_job_requested_cpu`
- `lsf_job_cpu_time_seconds`
- `lsf_job_requested_memory_kilobytes`
- `lsf_job_memory_kilobytes`
- `lsf_job_swap_kilobytes`
- `lsf_job_exit_status`
- `lsf_job_runtime_seconds`
- `lsf_jobs_total`

## External Resource JSON

Queue, host, cluster, license, GPU, and custom resource collection is exposed through an optional external JSON command. Set `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` to a local script or command that writes a JSON object with any of these fields: `jobs`, `queues`, `hosts`, `cluster`, `licenses`, and `custom_resources`.

This repository includes a helper script for common LSF CLI output:

```sh
chmod +x scripts/collect-extra.sh
LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND="./scripts/collect-extra.sh" ./lsf-exporter
```

The helper reads `lsid` for cluster/master, `bqueues` for queues, `bhosts` for hosts, `bmgroup -w` for host groups, `busers -w` for users, `bugroup -w` for user groups, `lsinfo -r` for resource definitions, and tries `blstat` first then `lmstat -a` for license features. If a command is unavailable, that section is returned empty and the diagnostic is written to stderr.

Host group relationships are included in JSON-only fields: each host can include `resources.host_groups`, each queue can include `raw.host_spec`, `raw.host_spec_tokens`, `raw.host_groups`, and `raw.hosts`, and each host group is also published as a `custom_resources` item with `type: "host_group"`. For queues, a token in the `bqueues -l` `HOSTS` field is treated as a host group when it contains `/`; the script strips `/`, runs `bhosts <group>`, and combines those members with single-host tokens into the de-duplicated `raw.hosts` set.

Queue user relationships are included in JSON-only fields: each queue can include `raw.user_spec`, `raw.user_spec_tokens`, `raw.user_groups`, and `raw.users`. For queues, a token in the `bqueues -l` `USERS` field is treated as a user group when it contains `/`; `all` is expanded to the users returned by `busers -w` when that command is available.

Queue interactivity is also JSON-only: `raw.interactive` is `true` when the queue detail from `bqueues -l` contains `INTERACTIVE`; `NO_INTERACTIVE` takes precedence and forces `raw.interactive` to `false`. `raw.interactive_source` keeps the matching detail line when present.

See `README.zh-CN.md` for the full JSON schema and examples.
## Prometheus Scrape Example

```yaml
scrape_configs:
  - job_name: lsf
    scrape_interval: 15s
    static_configs:
      - targets: ["lsf-exporter.example.com:9818"]
```

Set `scrape_interval` no lower than `LSF_EXPORTER_MIN_INTERVAL`. Scraping faster will not increase LSF API load, but it will not improve freshness either because `/metrics` reads the cached snapshot.



使用步骤：

  1. 在有 LSF 客户端/SDK 的机器上构建

  cd /Users/sunny/monitor/lsf-exporter
  go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter

  如果 LSF 头文件和库不在默认路径：

  CGO_CFLAGS="-I/path/to/lsf/include" \
  CGO_LDFLAGS="-L/path/to/lsf/lib" \
  go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter

  2. 启动 exporter

  LSF_EXPORTER_LISTEN_ADDRESS=":9818" \
  LSF_EXPORTER_INTERVAL="30s" \
  LSF_EXPORTER_MIN_INTERVAL="10s" \
  ./lsf-exporter

  3. 查看接口

  curl http://localhost:9818/healthz
  curl http://localhost:9818/metrics
  curl http://localhost:9818/jobs

  4. 配置 Prometheus

  scrape_configs:
    - job_name: lsf
      scrape_interval: 30s
      static_configs:
        - targets: ["your-lsf-exporter-host:9818"]

  默认只采集当前 active jobs，保护 LSF master。不要一开始打开全量历史 job。确实需要 finished/done/exit job 时再设置：

  LSF_EXPORTER_ALL_JOBS=true ./lsf-exporter

  建议生产先用这些保守配置：

  LSF_EXPORTER_INTERVAL=30s
  LSF_EXPORTER_MIN_INTERVAL=10s
  LSF_EXPORTER_ALL_JOBS=false
  LSF_EXPORTER_LOG_LEVEL=info
