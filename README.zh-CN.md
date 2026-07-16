# lsf-exporter 中文说明

`lsf-exporter` 是一个 IBM Spectrum LSF 的 Prometheus Exporter。它默认通过 LSF C API 周期性采集 job 信息，并支持通过外部 JSON 命令扩展 queue、host、cluster、license、GPU 和自定义 resource 等信息。

## 设计目标

- Prometheus scrape 不直接调用 LSF API。
- 所有 LSF 采集都在后台循环中完成。
- scrape 只读取内存快照，降低 LSF master 压力。
- 同一时间只允许一个采集任务运行。
- 支持最小采集间隔，避免过度访问 LSF。
- 高基数字段进入 JSON，不直接作为 Prometheus label。

## 构建

生产环境需要安装 IBM Spectrum LSF SDK，并使用 `lsf` tag 构建：

```sh
go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter
```

如果 LSF 头文件或库不在默认路径中，可以设置 cgo 参数：

```sh
CGO_CFLAGS="-I/path/to/lsf/include" \
CGO_LDFLAGS="-L/path/to/lsf/lib" \
go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter
```

不带 `-tags lsf` 的构建会启用 stub collector，仅用于开发和非 LSF 环境验证。

## 运行

```sh
./lsf-exporter
```

项目在 `deploy/` 下提供 systemd 部署文件：

```sh
install -D -m 0755 lsf-exporter /opt/lsf-exporter/lsf-exporter
install -D -m 0755 scripts/collect-extra.sh /opt/lsf-exporter/collect-extra.sh
install -D -m 0644 deploy/lsf-exporter.env.example /etc/sysconfig/lsf-exporter
install -D -m 0644 deploy/lsf-exporter.service /etc/systemd/system/lsf-exporter.service
systemctl daemon-reload
systemctl enable --now lsf-exporter
```

启动前请按现场 LSF 安装路径调整 `/etc/sysconfig/lsf-exporter`。

默认监听地址是 `:9818`。

## HTTP 接口

| 接口 | 说明 |
| --- | --- |
| `/metrics` | Prometheus text format 指标 |
| `/jobs` | 完整缓存快照，兼容旧接口 |
| `/all-jobs` | 独立的全量 job 查询缓存，带 `?refresh=true` 或 `?trigger=true` 时触发一次 `ALL_JOB` 查询 |
| `/snapshot` | 完整缓存快照 |
| `/queues` | queue 快照 |
| `/hosts` | host 快照 |
| `/cluster` | cluster/master 快照 |
| `/licenses` | license feature 快照 |
| `/resources` | 自定义 resource 快照 |
| `/healthz` | exporter 进程健康检查 |

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LSF_EXPORTER_LISTEN_ADDRESS` | `:9818` | HTTP 监听地址 |
| `LSF_EXPORTER_INTERVAL` | `30s` | 后台采集间隔 |
| `LSF_EXPORTER_MIN_INTERVAL` | `10s` | 最小 LSF API 采集间隔 |
| `LSF_EXPORTER_COLLECT_TIMEOUT` | `20s` | 采集耗时告警阈值；无法强制中断阻塞中的 LSF C 调用 |
| `LSF_EXPORTER_LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error` |
| `LSF_EXPORTER_APP_NAME` | `lsf-exporter` | 传给 `lsb_init` 的应用名 |
| `LSF_EXPORTER_QUERY_USER` | `all` | job 用户过滤 |
| `LSF_EXPORTER_QUERY_QUEUE` | 空 | job 队列过滤 |
| `LSF_EXPORTER_QUERY_HOST` | 空 | job 主机过滤 |
| `LSF_EXPORTER_QUERY_JOB_NAME` | 空 | job 名称过滤 |
| `LSF_EXPORTER_QUERY_JOB_ID` | `0` | 指定 job ID，`0` 表示不过滤 |
| `LSF_EXPORTER_ALL_JOBS` | `false` | 是否使用 `ALL_JOB` 查询历史 job |
| `LSF_EXPORTER_DISABLE_NATIVE_COLLECTOR` | `false` | 是否禁用原生 LSF C API collector，仅使用外部 JSON 采集 |
| `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` | 空 | 外部 JSON 扩展采集命令 |
| `LSF_EXPORTER_EXTERNAL_RESOURCE_TIMEOUT` | `10s` | 外部扩展命令超时时间 |

## 全量 Job 查询接口

`/jobs` 仍然是后台采集循环维护的常规缓存快照。`/all-jobs` 是单独的全量 job 查询缓存，用于人工或受控地读取历史 job，避免把全量历史 job 混入 Prometheus 使用的常规快照。

- `GET /all-jobs` 只返回上一次独立全量查询缓存，不调用 LSF。
- `GET /all-jobs?refresh=true` 执行一次原生 `ALL_JOB` 查询，并替换 `/all-jobs` 缓存。
- `GET /all-jobs?trigger=true` 与 `refresh=true` 等价。

响应会额外包含 `scope: "all_jobs"` 和 `refreshed` 字段，调用方可以明确区分它和 `/jobs`。刷新操作同一时间只允许一个执行，并复用 `LSF_EXPORTER_MIN_INTERVAL` 做限流；并发刷新返回 `409`，触发过快返回 `429`。

## 原生 Job 采集

使用 `-tags lsf` 构建后，原生 collector 调用以下 LSF C API：

| API | 用途 |
| --- | --- |
| `lsb_init` | 初始化 LSF batch API |
| `lsb_openjobinfo` | 打开 job 查询 |
| `lsb_readjobinfo` | 逐条读取 job 信息 |
| `lsb_closejobinfo` | 关闭 job 查询资源 |

当前原生 C API 路径会采集以下 job 字段：

| 类别 | 字段 |
| --- | --- |
| 标识 | `id`、`array_index` |
| 基础信息 | `name`、`user`、`queue`、`status`、`status_code` |
| 项目应用 | `project`、`application`、`service_class` |
| 主机 | `from_host`、`execution_host` |
| 命令路径 | `command`、`cwd`、`input_file`、`output_file`、`error_file` |
| 时间 | `submit_time`、`start_time`、`end_time` |
| 退出 | `exit_status` |
| 资源 | `requested_cpu`、`cpu_time_seconds`、`requested_memory_kb`、`memory_kb`、`swap_kb` |
| 提交表达式 | `resource_requirement`、`dependency_condition` |

`requested_cpu` 表示提交时请求的 CPU/slot 数，`cpu_time_seconds` 表示 LSF 返回的累计 CPU 使用时间。`requested_memory_kb` 从 `rusage[mem=...]` 解析得到，`memory_kb` 表示 job 实际内存使用。

`resource_requirement`、`dependency_condition`、命令和路径属于高基数字段，只放在 JSON 快照中，不作为 Prometheus label。

## 外部 JSON 扩展采集

queue、host、cluster、license、GPU、自定义 resource 等能力通过 `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` 接入。命令需要向标准输出写出 JSON，结构如下：

项目内提供了一个基于常见 LSF CLI 输出的辅助脚本：

```sh
chmod +x scripts/collect-extra.sh
LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND="./scripts/collect-extra.sh" ./lsf-exporter
```

该脚本通过 `lsid` 获取 cluster/master，通过 `bqueues` 获取 queue，通过 `bhosts` 获取 host，通过 `bmgroup -w` 获取机器组，通过 `busers -w` 获取用户，通过 `bugroup -w` 获取用户组，通过 `lsinfo -r` 获取 resource 定义，并优先用 `blstat`、其次用 `lmstat -a` 尝试获取 license feature。某个命令不可用时，对应部分会返回空，并把诊断信息写到 stderr。

机器组关联会放在 JSON 字段中：host 会包含 `resources.host_groups`，queue 会包含 `raw.host_spec`、`raw.host_spec_tokens`、`raw.host_groups` 和 `raw.hosts`，每个机器组也会以 `type: "host_group"` 的 `custom_resources` 条目输出。对 queue 来说，`bqueues -l` 的 `HOSTS` 字段中带 `/` 的 token 会被当作机器组；脚本去除 `/` 后执行 `bhosts <group>` 展开成员，并把这些成员与不带 `/` 的单节点 host 合并去重后写入 `raw.hosts`。

用户关联会放在 JSON 字段中：queue 会包含 `raw.user_spec`、`raw.user_spec_tokens`、`raw.user_groups` 和 `raw.users`。对 queue 来说，`bqueues -l` 的 `USERS` 字段中带 `/` 的 token 会被当作用户组；`all` 会在 `busers -w` 可用时展开为全部用户。

queue 是否 interactive 也放在 JSON 字段中：当 `bqueues -l` 的队列详情包含 `INTERACTIVE` 时，`raw.interactive` 为 `true`；`NO_INTERACTIVE` 优先级更高，会强制 `raw.interactive` 为 `false`。如果匹配到对应行，会放到 `raw.interactive_source` 便于核对。

```json
{
  "queues": [
    {
      "name": "normal",
      "status": "Open:Active",
      "priority": 30,
      "open": true,
      "active": true,
      "max_jobs": 1000,
      "num_jobs": 120,
      "pending": 20,
      "running": 95,
      "suspended": 5
    }
  ],
  "hosts": [
    {
      "name": "host01",
      "status": "ok",
      "closed": false,
      "max_jobs": 64,
      "num_jobs": 20,
      "running": 18,
      "suspended": 2,
      "load": {
        "r15s": 1.2,
        "mem": 204800
      },
      "resources": {
        "gpu_model": "A100"
      }
    }
  ],
  "cluster": {
    "name": "prod-lsf",
    "master": "master01",
    "status": "ok",
    "master_up": true
  },
  "licenses": [
    {
      "feature": "feature-a",
      "total": 100,
      "used": 42,
      "free": 58
    }
  ],
  "custom_resources": [
    {
      "name": "gpu",
      "type": "numeric",
      "location": "cluster",
      "total": 128,
      "used": 64,
      "free": 64
    }
  ],
  "jobs": [
    {
      "id": 12345,
      "pending_reason": "waiting for resource",
      "exit_reason": "completed",
      "gpu_requested": 2,
      "gpu_used": 1
    }
  ]
}
```

如果外部 JSON 也返回 `jobs`，这些 job 会追加到原生 LSF C API 返回的 job 列表中。建议外部 job 只用于补充 pending reason、exit reason、GPU 等原生 collector 暂未映射的信息，避免重复输出完整 job。

Windows 示例：

```powershell
$env:LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND = "powershell.exe -NoProfile -File D:\lsf\collect-extra.ps1"
.\lsf-exporter.exe
```

Linux 示例：

```sh
LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND="/opt/lsf-exporter/collect-extra.sh" ./lsf-exporter
```

## Prometheus 指标

Exporter 自监控指标：

| 指标 | 说明 |
| --- | --- |
| `lsf_exporter_up` | 最近一次采集是否成功 |
| `lsf_exporter_collections_total` | 总采集次数 |
| `lsf_exporter_collect_errors_total` | 采集失败次数 |
| `lsf_exporter_collect_skipped_total` | 跳过采集次数 |
| `lsf_exporter_snapshot_jobs` | 当前 job 数 |
| `lsf_exporter_snapshot_queues` | 当前 queue 数 |
| `lsf_exporter_snapshot_hosts` | 当前 host 数 |
| `lsf_exporter_snapshot_licenses` | 当前 license feature 数 |
| `lsf_exporter_snapshot_custom_resources` | 当前自定义 resource 数 |
| `lsf_exporter_last_success_timestamp_seconds` | 最近成功采集时间 |
| `lsf_exporter_snapshot_age_seconds` | 当前快照年龄 |
| `lsf_exporter_last_collect_duration_seconds` | 最近一次采集耗时 |

Job 指标：

| 指标 | 说明 |
| --- | --- |
| `lsf_job_info` | job 信息，值固定为 `1` |
| `lsf_job_requested_cpu` | job 提交时请求的 CPU/slot 数 |
| `lsf_job_cpu_time_seconds` | job CPU time |
| `lsf_job_requested_memory_kilobytes` | job 提交时请求的内存 |
| `lsf_job_memory_kilobytes` | job 内存使用 |
| `lsf_job_swap_kilobytes` | job swap 使用 |
| `lsf_job_exit_status` | job 退出状态码 |
| `lsf_job_gpu_requested` | job GPU 请求量，来自外部 JSON |
| `lsf_job_gpu_used` | job GPU 使用量，来自外部 JSON |
| `lsf_job_runtime_seconds` | job 运行时长 |
| `lsf_jobs_total` | 按状态统计的 job 数 |

Queue 指标：

| 指标 | 说明 |
| --- | --- |
| `lsf_queue_info` | queue 信息 |
| `lsf_queue_priority` | queue 优先级 |
| `lsf_queue_jobs` | queue 内 job 数，按 `state` 区分 |
| `lsf_queue_max_jobs` | queue 最大 job 数 |
| `lsf_queue_open` | queue 是否 open |
| `lsf_queue_active` | queue 是否 active |

Host 指标：

| 指标 | 说明 |
| --- | --- |
| `lsf_host_info` | host 信息 |
| `lsf_host_jobs` | host 上 job 数，按 `state` 区分 |
| `lsf_host_max_jobs` | host 最大 job 数 |
| `lsf_host_closed` | host 是否关闭 |
| `lsf_host_load` | host load/resource 数值 |

Cluster、license 和自定义 resource 指标：

| 指标 | 说明 |
| --- | --- |
| `lsf_cluster_info` | cluster/master 信息 |
| `lsf_cluster_master_up` | master 是否可用 |
| `lsf_license_total` | license 总量 |
| `lsf_license_used` | license 已用量 |
| `lsf_license_free` | license 空闲量 |
| `lsf_custom_resource_total` | 自定义 resource 总量 |
| `lsf_custom_resource_used` | 自定义 resource 已用量 |
| `lsf_custom_resource_free` | 自定义 resource 空闲量 |

## Prometheus 配置示例

```yaml
scrape_configs:
  - job_name: lsf
    scrape_interval: 30s
    static_configs:
      - targets: ["lsf-exporter.example.com:9818"]
```

建议 `scrape_interval` 不低于 `LSF_EXPORTER_MIN_INTERVAL`。更高频 scrape 只会读取缓存，不会提升数据新鲜度。

## 生产建议

保守启动配置：

```sh
LSF_EXPORTER_INTERVAL=30s
LSF_EXPORTER_MIN_INTERVAL=10s
LSF_EXPORTER_ALL_JOBS=false
LSF_EXPORTER_LOG_LEVEL=info
```

只有在确认历史 job 数量和 LSF master 压力可控后，再启用：

```sh
LSF_EXPORTER_ALL_JOBS=true
```

外部 JSON 命令应由本地脚本完成 LSF CLI/API 调用，并控制输出字段和维度，避免把长文本、高基数字段暴露为 Prometheus label。
