# LSF Exporter 功能梳理

## 项目定位

本项目是一个面向 IBM Spectrum LSF 的 Prometheus Exporter。它通过 LSF C API 周期性读取 LSF 作业信息，将结果缓存到内存中，再通过 HTTP 接口提供给 Prometheus 或其他调用方查询。

项目重点不是实时同步所有 LSF 对象，而是以较低风险的方式暴露 LSF job 信息，避免 Prometheus 高频 scrape 直接压到 LSF master。

## 总体架构

程序启动后会执行以下流程：

1. 读取环境变量配置。
2. 初始化 LSF collector。
3. 启动后台采集循环。
4. 定期调用 LSF C API 获取 job 信息。
5. 将 job 信息复制到 Go 内存快照。
6. 启动 HTTP 服务，对外提供 `/metrics`、`/jobs`、`/healthz`。

生产环境需要使用 `lsf` build tag 构建：

```sh
go build -tags lsf -o lsf-exporter ./cmd/lsf-exporter
```

未使用 `-tags lsf` 构建时，会启用 stub collector。该模式不会连接 LSF，只会返回提示错误，主要用于没有 LSF SDK 的开发或测试环境。

## LSF API 使用情况

生产版 collector 使用以下 LSF C API：

| API | 用途 |
| --- | --- |
| `lsb_init` | 初始化 LSF batch API |
| `lsb_openjobinfo` | 打开 job 查询 |
| `lsb_readjobinfo` | 逐条读取 job 信息 |
| `lsb_closejobinfo` | 关闭 job 查询资源 |

每次 `lsb_openjobinfo` 成功后，代码都会在对应路径上调用 `lsb_closejobinfo`，避免泄露 LSF 查询资源。

## 可获取的 LSF 信息

当前项目只采集 LSF job 信息，不采集 queue、host、cluster、license、mbatchd 状态等其他 LSF 对象。

### Job 基础信息

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| Job ID | `id` | LSF 作业 ID |
| Array Index | `array_index` | 作业数组下标，当前 C 结构中预留但未从 LSF 字段实际赋值 |
| Job Name | `name` | 作业名称 |
| User | `user` | 提交用户 |
| Queue | `queue` | 提交队列 |
| Status | `status` | 作业状态文本 |
| Status Code | `status_code` | LSF 原始状态码 |

### 项目与应用信息

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| Project | `project` | LSF project name |
| Application | `application` | LSF application profile |
| Service Class | `service_class` | SLA/service class |

### 主机信息

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| From Host | `from_host` | 提交来源主机 |
| Execution Host | `execution_host` | 执行主机列表，多个主机用逗号拼接 |

### 命令与文件路径

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| Command | `command` | 作业提交命令 |
| Current Working Directory | `cwd` | 作业工作目录 |
| Input File | `input_file` | 标准输入文件 |
| Output File | `output_file` | 标准输出文件 |
| Error File | `error_file` | 标准错误文件 |

### 时间信息

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| Submit Time | `submit_time` | 作业提交时间，Unix 秒 |
| Start Time | `start_time` | 作业开始时间，Unix 秒 |
| End Time | `end_time` | 作业结束时间，Unix 秒 |
| Exit Status | `exit_status` | LSF 返回的作业退出状态码 |

### 资源使用信息

| 字段 | JSON 字段 | 说明 |
| --- | --- | --- |
| CPU Time | `cpu_time_seconds` | LSF 返回的 CPU time，单位秒 |
| Memory | `memory_kb` | 运行资源使用中的内存值，单位 KB |
| Swap | `swap_kb` | 运行资源使用中的 swap 值，单位 KB |
| Resource Requirement | `resource_requirement` | 作业提交时的资源请求表达式 |
| Dependency Condition | `dependency_condition` | 作业提交时的依赖条件表达式 |

### Raw 字段

`/jobs` JSON 中每个 job 还包含 `raw` map，用字符串形式重复保存主要字段，便于排查或兼容简单消费端。

## 支持的 Job 状态

代码会将 LSF job status 位转换为以下文本：

| 状态 | 含义 |
| --- | --- |
| `PEND` | 等待调度 |
| `RUN` | 正在运行 |
| `PSUSP` | 被管理员或系统策略挂起 |
| `USUSP` | 被用户挂起 |
| `SSUSP` | 被系统挂起 |
| `DONE` | 正常完成 |
| `EXIT` | 异常退出 |
| `UNKWN` | 状态未知 |
| `WAIT` | 等待 |
| `UNKNOWN` | 未匹配到已知状态 |

## 采集范围与过滤条件

默认情况下，项目使用 `CUR_JOB` 查询当前作业，避免默认查询全量历史作业。

如果设置：

```sh
LSF_EXPORTER_ALL_JOBS=true
```

则会使用 `ALL_JOB` 查询，可能包含 `DONE`、`EXIT` 等历史作业。该模式可能返回大量数据，应在评估 LSF master 压力后再启用。

支持以下查询过滤条件：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LSF_EXPORTER_QUERY_USER` | `all` | 用户过滤 |
| `LSF_EXPORTER_QUERY_QUEUE` | 空 | 队列过滤 |
| `LSF_EXPORTER_QUERY_HOST` | 空 | 主机过滤 |
| `LSF_EXPORTER_QUERY_JOB_NAME` | 空 | 作业名过滤 |
| `LSF_EXPORTER_QUERY_JOB_ID` | `0` | 指定 job ID，`0` 表示不过滤 |
| `LSF_EXPORTER_ALL_JOBS` | `false` | 是否查询所有 job |

## HTTP 接口

| 接口 | 说明 |
| --- | --- |
| `/metrics` | Prometheus text format 指标 |
| `/jobs` | 当前缓存 job 快照，JSON 格式 |
| `/healthz` | 健康检查，返回 `ok` |

### `/jobs` 返回结构

`/jobs` 返回的是当前内存快照，结构如下：

```json
{
  "jobs": [
    {
      "id": 12345,
      "name": "job-name",
      "user": "alice",
      "queue": "normal",
      "status": "RUN",
      "status_code": 32,
      "project": "project-a",
      "application": "app-a",
      "service_class": "sla-a",
      "from_host": "submit-host",
      "execution_host": "exec-host-1,exec-host-2",
      "command": "run.sh",
      "cwd": "/work/path",
      "input_file": "input.txt",
      "output_file": "output.txt",
      "error_file": "error.txt",
      "submit_time": 1710000000,
      "start_time": 1710000100,
      "end_time": 0,
      "exit_status": 0,
      "cpu_time_seconds": 123.4,
      "memory_kb": 204800,
      "swap_kb": 0,
      "resource_requirement": "select[type==X86_64] rusage[mem=1024]",
      "dependency_condition": "done(12344)"
    }
  ],
  "collected_at": "2026-07-06T10:00:00Z",
  "duration": "123ms"
}
```

如果最近一次采集失败，快照中会包含 `error` 字段。

## Prometheus 指标

### Exporter 自监控指标

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `lsf_exporter_up` | gauge | 最近一次 LSF 采集是否成功，成功为 `1`，失败为 `0` |
| `lsf_exporter_collections_total` | counter | 总采集次数 |
| `lsf_exporter_collect_errors_total` | counter | 采集失败次数 |
| `lsf_exporter_collect_skipped_total` | counter | 因采集间隔或并发保护跳过的次数 |
| `lsf_exporter_snapshot_jobs` | gauge | 当前快照中的 job 数量 |
| `lsf_exporter_last_success_timestamp_seconds` | gauge | 最近一次成功采集时间，Unix 秒 |
| `lsf_exporter_snapshot_age_seconds` | gauge | 当前快照年龄，单位秒 |
| `lsf_exporter_last_collect_duration_seconds` | gauge | 最近一次采集耗时，单位秒 |

### Job 指标

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `lsf_job_info` | gauge | job 基础信息，值固定为 `1` |
| `lsf_job_cpu_time_seconds` | gauge | job CPU time，单位秒 |
| `lsf_job_memory_kilobytes` | gauge | job 内存使用，单位 KB |
| `lsf_job_swap_kilobytes` | gauge | job swap 使用，单位 KB |
| `lsf_job_exit_status` | gauge | job 退出状态码 |
| `lsf_job_runtime_seconds` | gauge | job 运行时长，单位秒 |
| `lsf_jobs_total` | gauge | 按状态统计的 job 数量 |

`lsf_job_info` 标签包括：

| Label | 说明 |
| --- | --- |
| `job_id` | Job ID |
| `user` | 用户 |
| `queue` | 队列 |
| `status` | 状态 |
| `job_name` | 作业名 |
| `project` | 项目 |
| `application` | 应用 |
| `execution_host` | 执行主机 |

资源类 job 指标主要使用以下标签：

| Label | 说明 |
| --- | --- |
| `job_id` | Job ID |
| `status` | 状态 |

## LSF Master 保护机制

项目采用缓存型采集模型，Prometheus scrape 不会直接调用 LSF API。

关键保护机制如下：

1. 只有后台采集循环会访问 LSF。
2. `/metrics` 和 `/jobs` 只读取内存快照。
3. 同一时间只允许一个采集任务运行。
4. 如果上一次采集尚未完成，新一轮采集会被跳过。
5. 如果距离上次采集小于 `LSF_EXPORTER_MIN_INTERVAL`，采集会被跳过。
6. `LSF_EXPORTER_INTERVAL` 不能小于 `LSF_EXPORTER_MIN_INTERVAL`。
7. 默认不查询全量历史 job。

## 运行配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LSF_EXPORTER_LISTEN_ADDRESS` | `:9818` | HTTP 监听地址 |
| `LSF_EXPORTER_INTERVAL` | `30s` | 后台采集间隔 |
| `LSF_EXPORTER_MIN_INTERVAL` | `10s` | 最小 LSF API 采集间隔 |
| `LSF_EXPORTER_COLLECT_TIMEOUT` | `20s` | 采集耗时告警阈值；Go 无法安全中断阻塞中的 LSF C 调用 |
| `LSF_EXPORTER_LOG_LEVEL` | `info` | 日志级别，支持 `debug`、`info`、`warn`、`error` |
| `LSF_EXPORTER_APP_NAME` | `lsf-exporter` | 传给 `lsb_init` 的应用名 |

## 当前不支持的信息

当前原生 LSF C API collector 仍不直接提供以下信息；这些信息现在可以通过 `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` 返回 JSON 后进入快照和 Prometheus 指标：

| 信息 | 当前状态 |
| --- | --- |
| LSF queue 配置与负载 | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| LSF host 状态、slot、load | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| LSF cluster/master 状态 | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| license 使用情况 | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| pending reason | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| exit reason 详细文本 | 外部 JSON 扩展支持；原生 C API collector 已采集 `exit_status` |
| job dependency | 部分采集；当前已采集提交时的 `dependency_condition`，但未采集依赖解析诊断 |
| resource requirement 详细内容 | 部分采集；当前已采集提交时的 `resource_requirement` 字符串 |
| GPU 使用情况 | 外部 JSON 扩展支持；原生 C API collector 未采集 |
| 自定义 resource 使用情况 | 外部 JSON 扩展支持；原生 C API collector 未采集 |

如果后续需要完全原生化这些能力，可以在当前 `Source` 接口后继续扩展 LSF C API collector，替换或补充外部 JSON 扩展命令。

## 不支持信息的扩展规划

以下内容基于原始设计文档的边界：当前 exporter 的采集对象是 job，且 Prometheus scrape 路径不能直接调用 LSF API。新增能力应保持后台采集、内存快照、低基数指标的设计原则。

| 信息类别 | 当前不支持的具体内容 | 不支持原因 | 建议扩展方式 | 推荐暴露方式 |
| --- | --- | --- | --- | --- |
| Queue 信息 | 队列配置、队列状态、队列 pending/running 数、队列级 slot 使用 | 当前只调用 job 查询 API，没有队列 collector | 新增独立 queue collector，使用单独采集间隔和快照 | 聚合指标进 `/metrics`，详细配置进 JSON |
| Host 信息 | host 状态、slot、load、关闭状态、资源可用量 | host 数据变化频率和容量模型与 job 不同，直接放入 job 标签会造成高基数 | 新增 host collector，不与 job collector 强耦合 | host 聚合指标进 `/metrics`，详细 host 快照进 JSON |
| Cluster/Master 状态 | master 是否可用、mbatchd 状态、集群健康 | 当前 `/healthz` 只代表 exporter 进程可用，不代表 LSF 控制面健康 | 新增 cluster health collector 或轻量探测 API | `lsf_cluster_up`、`lsf_master_up` 等低基数指标 |
| License 信息 | license 总量、已用量、特性维度使用量 | license 信息不在 `jobInfoEnt` 字段内 | 如果目标环境提供 LSF license API，新增 license collector | 按 feature 输出聚合指标，避免用户/job 级标签 |
| Pending Reason | pending 原因、调度阻塞原因、资源不足原因 | 当前只映射 job status，没有复制 pending reason 诊断字段 | 扩展 job 字段映射，必要时增加诊断查询 | 详细文本放 `/jobs`，Prometheus 只做原因分类计数 |
| Exit Reason | 退出原因、终止信号、调度系统终止原因 | 当前已暴露 `exit_status`，但缺少详细终止诊断字段 | 确认稳定 LSF 字段后扩展 `jobInfoEnt` 映射 | 详细原因放 `/jobs`，Prometheus 输出分类统计 |
| Job Dependency | 依赖未满足原因、依赖解析状态 | 当前已暴露提交时的 `dependency_condition`，但没有依赖诊断 | 后续补充依赖解析结果或诊断字段 | 不建议作为 label；可按是否存在依赖做聚合指标 |
| Resource Requirement | 解析后的 `rusage`、`select`、`span`、`order` 维度 | 当前已暴露原始 `resource_requirement` 字符串，但未解析为结构化字段 | 后续按白名单解析低基数维度 | JSON 优先；Prometheus 只输出解析后的有限分类 |
| GPU 信息 | GPU 请求量、GPU 使用量、GPU 型号、GPU host 分布 | LSF GPU 字段和站点配置强相关，当前未做映射 | 基于实际 LSF 版本和集群资源定义增加字段或解析逻辑 | 先做 JSON，再对核心维度做 allowlist 指标 |
| 自定义 Resource | 自定义资源请求、使用、可用量 | 不同集群差异很大，直接暴露会造成不可控指标维度 | 通过配置 allowlist 控制哪些 resource 可暴露 | allowlist 后输出聚合指标，原始详情放 JSON |

扩展优先级建议如下：

1. 继续补充 job 详情字段，例如 pending reason、exit reason 详细文本、资源请求解析结果。这类能力仍围绕现有 job collector，改动边界较小。
2. 再新增 queue 和 host collector。这两类数据适合独立快照和独立采集间隔，避免影响现有 job 采集稳定性。
3. 最后考虑 license、GPU、自定义 resource。这些能力通常依赖具体 LSF 版本、站点配置和业务口径，需要先确定字段来源和指标基数控制策略。

Prometheus 暴露原则：

1. 数值型、低基数字段可以进入 `/metrics`。
2. 长文本、命令、路径、原因详情、资源表达式优先进入 `/jobs` 或新的 JSON 详情接口。
3. 不应把 command、cwd、pending reason、resource requirement、dependency expression 等长文本作为 Prometheus label。
4. 自定义 resource、GPU 类型、license feature 等维度应通过配置白名单控制。
## 外部 JSON 扩展能力

新增配置 `LSF_EXPORTER_EXTERNAL_RESOURCE_COMMAND` 后，exporter 会执行该命令并解析标准输出中的 JSON。该 JSON 可以包含 `jobs`、`queues`、`hosts`、`cluster`、`licenses`、`custom_resources`，用于补充原生 C API collector 暂未覆盖的 LSF 信息。

这一路径适合接入站点已有的 LSF CLI 脚本、内部 API 或后续独立采集程序。它仍遵守原设计原则：命令只在后台采集循环中执行，Prometheus scrape 仅读取缓存快照。
## 使用建议

生产环境建议从保守配置开始：

```sh
LSF_EXPORTER_INTERVAL=30s
LSF_EXPORTER_MIN_INTERVAL=10s
LSF_EXPORTER_ALL_JOBS=false
LSF_EXPORTER_LOG_LEVEL=info
```

Prometheus 的 `scrape_interval` 建议不低于 `LSF_EXPORTER_MIN_INTERVAL`。即使 Prometheus 更高频 scrape，也只会读取缓存快照，不会提高数据新鲜度。

如需查询历史完成作业，再评估启用：

```sh
LSF_EXPORTER_ALL_JOBS=true
```

启用前应确认历史 job 数量和 LSF master 压力，避免一次采集返回过多数据。
