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

Endpoints:

- `/metrics`: Prometheus metrics.
- `/jobs`: cached job snapshot as JSON, including copied job fields.
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

## Prometheus Metrics

Important self-monitoring metrics:

- `lsf_exporter_up`
- `lsf_exporter_collections_total`
- `lsf_exporter_collect_errors_total`
- `lsf_exporter_collect_skipped_total`
- `lsf_exporter_last_success_timestamp_seconds`
- `lsf_exporter_snapshot_age_seconds`
- `lsf_exporter_last_collect_duration_seconds`

Job metrics:

- `lsf_job_info`
- `lsf_job_cpu_time_seconds`
- `lsf_job_memory_kilobytes`
- `lsf_job_swap_kilobytes`
- `lsf_job_runtime_seconds`
- `lsf_jobs_total`

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