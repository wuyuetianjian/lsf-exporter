package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"lsf-exporter/internal/collector"
	"lsf-exporter/internal/logger"
)

func Register(mux *http.ServeMux, svc *collector.Service, fullJobs *collector.JobQueryService, log *logger.Logger) {
	mux.HandleFunc("/metrics", metricsHandler(svc))
	mux.HandleFunc("/snapshot", snapshotHandler(svc, log))
	mux.HandleFunc("/jobs", jobsHandler(svc, log))
	mux.HandleFunc("/all-jobs", allJobsHandler(fullJobs, log))
	mux.HandleFunc("/queues", queuesHandler(svc, log))
	mux.HandleFunc("/hosts", hostsHandler(svc, log))
	mux.HandleFunc("/cluster", clusterHandler(svc, log))
	mux.HandleFunc("/licenses", licensesHandler(svc, log))
	mux.HandleFunc("/resources", resourcesHandler(svc, log))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}

func metricsHandler(svc *collector.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		snap := svc.Snapshot()
		stats := svc.Stats()
		var b strings.Builder

		up := 1.0
		if snap.Error != "" {
			up = 0
		}
		writeHelp(&b, "lsf_exporter_up", "Whether the last LSF collection succeeded.")
		writeGauge(&b, "lsf_exporter_up", nil, up)
		writeCounter(&b, "lsf_exporter_collections_total", nil, float64(stats.Collections))
		writeCounter(&b, "lsf_exporter_collect_errors_total", nil, float64(stats.Errors))
		writeCounter(&b, "lsf_exporter_collect_skipped_total", nil, float64(stats.Skipped))
		writeGauge(&b, "lsf_exporter_snapshot_jobs", nil, float64(len(snap.Jobs)))
		writeGauge(&b, "lsf_exporter_snapshot_queues", nil, float64(len(snap.Queues)))
		writeGauge(&b, "lsf_exporter_snapshot_hosts", nil, float64(len(snap.Hosts)))
		writeGauge(&b, "lsf_exporter_snapshot_licenses", nil, float64(len(snap.Licenses)))
		writeGauge(&b, "lsf_exporter_snapshot_custom_resources", nil, float64(len(snap.CustomResources)))
		if !stats.LastSuccess.IsZero() {
			writeGauge(&b, "lsf_exporter_last_success_timestamp_seconds", nil, float64(stats.LastSuccess.Unix()))
		}
		if !snap.CollectedAt.IsZero() {
			writeGauge(&b, "lsf_exporter_snapshot_age_seconds", nil, time.Since(snap.CollectedAt).Seconds())
		}
		if d, err := time.ParseDuration(snap.Duration); err == nil {
			writeGauge(&b, "lsf_exporter_last_collect_duration_seconds", nil, d.Seconds())
		}

		writeJobMetrics(&b, snap)
		writeQueueMetrics(&b, snap.Queues)
		writeHostMetrics(&b, snap.Hosts)
		writeClusterMetrics(&b, snap.Cluster)
		writeLicenseMetrics(&b, snap.Licenses)
		writeCustomResourceMetrics(&b, snap.CustomResources)

		_, _ = w.Write([]byte(b.String()))
	}
}

func writeJobMetrics(b *strings.Builder, snap collector.Snapshot) {
	byStatus := map[string]int{}
	now := time.Now().Unix()
	for _, job := range snap.Jobs {
		jobID := strconv.FormatInt(job.ID, 10)
		byStatus[job.Status]++
		writeGauge(b, "lsf_job_info", labels{
			"job_id":         jobID,
			"user":           job.User,
			"queue":          job.Queue,
			"status":         job.Status,
			"job_name":       job.Name,
			"project":        job.Project,
			"application":    job.Application,
			"execution_host": job.ExecutionHost,
		}, 1)
		if job.CPUTime > 0 {
			writeGauge(b, "lsf_job_cpu_time_seconds", labels{"job_id": jobID, "status": job.Status}, job.CPUTime)
		}
		if job.RequestedCPU > 0 {
			writeGauge(b, "lsf_job_requested_cpu", labels{"job_id": jobID, "status": job.Status}, float64(job.RequestedCPU))
		}
		if job.MemoryKB > 0 {
			writeGauge(b, "lsf_job_memory_kilobytes", labels{"job_id": jobID, "status": job.Status}, float64(job.MemoryKB))
		}
		if job.RequestedMemKB > 0 {
			writeGauge(b, "lsf_job_requested_memory_kilobytes", labels{"job_id": jobID, "status": job.Status}, float64(job.RequestedMemKB))
		}
		if job.SwapKB > 0 {
			writeGauge(b, "lsf_job_swap_kilobytes", labels{"job_id": jobID, "status": job.Status}, float64(job.SwapKB))
		}
		if job.EndTime > 0 || job.ExitStatus != 0 {
			writeGauge(b, "lsf_job_exit_status", labels{"job_id": jobID, "status": job.Status}, float64(job.ExitStatus))
		}
		if job.GPURequested > 0 {
			writeGauge(b, "lsf_job_gpu_requested", labels{"job_id": jobID, "status": job.Status}, job.GPURequested)
		}
		if job.GPUUsed > 0 {
			writeGauge(b, "lsf_job_gpu_used", labels{"job_id": jobID, "status": job.Status}, job.GPUUsed)
		}
		if job.StartTime > 0 {
			end := job.EndTime
			if end <= 0 {
				end = now
			}
			if end > job.StartTime {
				writeGauge(b, "lsf_job_runtime_seconds", labels{"job_id": jobID, "status": job.Status}, float64(end-job.StartTime))
			}
		}
	}
	for status, count := range byStatus {
		writeGauge(b, "lsf_jobs_total", labels{"status": status}, float64(count))
	}
}

func writeQueueMetrics(b *strings.Builder, queues []collector.Queue) {
	for _, queue := range queues {
		writeGauge(b, "lsf_queue_info", labels{"queue": queue.Name, "status": queue.Status}, 1)
		writeGauge(b, "lsf_queue_priority", labels{"queue": queue.Name}, float64(queue.Priority))
		writeGauge(b, "lsf_queue_jobs", labels{"queue": queue.Name, "state": "total"}, float64(queue.NumJobs))
		writeGauge(b, "lsf_queue_jobs", labels{"queue": queue.Name, "state": "pending"}, float64(queue.Pending))
		writeGauge(b, "lsf_queue_jobs", labels{"queue": queue.Name, "state": "running"}, float64(queue.Running))
		writeGauge(b, "lsf_queue_jobs", labels{"queue": queue.Name, "state": "suspended"}, float64(queue.Suspended))
		if queue.MaxJobs > 0 {
			writeGauge(b, "lsf_queue_max_jobs", labels{"queue": queue.Name}, float64(queue.MaxJobs))
		}
		if queue.Open != nil {
			writeGauge(b, "lsf_queue_open", labels{"queue": queue.Name}, boolFloat(*queue.Open))
		}
		if queue.Active != nil {
			writeGauge(b, "lsf_queue_active", labels{"queue": queue.Name}, boolFloat(*queue.Active))
		}
	}
}

func writeHostMetrics(b *strings.Builder, hosts []collector.Host) {
	for _, host := range hosts {
		writeGauge(b, "lsf_host_info", labels{"host": host.Name, "status": host.Status}, 1)
		writeGauge(b, "lsf_host_jobs", labels{"host": host.Name, "state": "total"}, float64(host.NumJobs))
		writeGauge(b, "lsf_host_jobs", labels{"host": host.Name, "state": "running"}, float64(host.Running))
		writeGauge(b, "lsf_host_jobs", labels{"host": host.Name, "state": "suspended"}, float64(host.Suspended))
		if host.MaxJobs > 0 {
			writeGauge(b, "lsf_host_max_jobs", labels{"host": host.Name}, float64(host.MaxJobs))
		}
		if host.Closed != nil {
			writeGauge(b, "lsf_host_closed", labels{"host": host.Name}, boolFloat(*host.Closed))
		}
		for name, value := range host.Load {
			writeGauge(b, "lsf_host_load", labels{"host": host.Name, "resource": name}, value)
		}
	}
}

func writeClusterMetrics(b *strings.Builder, cluster *collector.Cluster) {
	if cluster == nil {
		return
	}
	writeGauge(b, "lsf_cluster_info", labels{"cluster": cluster.Name, "master": cluster.Master, "status": cluster.Status}, 1)
	if cluster.MasterUp != nil {
		writeGauge(b, "lsf_cluster_master_up", labels{"cluster": cluster.Name, "master": cluster.Master}, boolFloat(*cluster.MasterUp))
	}
}

func writeLicenseMetrics(b *strings.Builder, licenses []collector.LicenseFeature) {
	for _, feature := range licenses {
		writeGauge(b, "lsf_license_total", labels{"feature": feature.Feature}, feature.Total)
		writeGauge(b, "lsf_license_used", labels{"feature": feature.Feature}, feature.Used)
		writeGauge(b, "lsf_license_free", labels{"feature": feature.Feature}, feature.Free)
	}
}

func writeCustomResourceMetrics(b *strings.Builder, resources []collector.CustomResource) {
	for _, resource := range resources {
		labels := labels{"resource": resource.Name, "type": resource.Type, "location": resource.Location}
		writeGauge(b, "lsf_custom_resource_total", labels, resource.Total)
		writeGauge(b, "lsf_custom_resource_used", labels, resource.Used)
		writeGauge(b, "lsf_custom_resource_free", labels, resource.Free)
	}
}

func snapshotHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot() })
}

func jobsHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot() })
}

type allJobsResponse struct {
	Scope     string             `json:"scope"`
	Refreshed bool               `json:"refreshed"`
	Error     string             `json:"error,omitempty"`
	Snapshot  collector.Snapshot `json:"snapshot"`
}

func allJobsHandler(svc *collector.JobQueryService, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if svc == nil {
			http.Error(w, `{"error":"native LSF job collector is disabled"}`, http.StatusServiceUnavailable)
			return
		}

		refresh, err := refreshRequested(r)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}

		snap := svc.Snapshot()
		refreshed := false
		status := http.StatusOK
		var responseErr string
		if refresh {
			refreshed = true
			snap, err = svc.CollectAllJobs()
			switch {
			case err == nil:
			case errors.Is(err, collector.ErrJobQueryInProgress):
				refreshed = false
				responseErr = err.Error()
				status = http.StatusConflict
			case errors.Is(err, collector.ErrJobQueryTooSoon):
				refreshed = false
				responseErr = err.Error()
				status = http.StatusTooManyRequests
			default:
				responseErr = err.Error()
				status = http.StatusInternalServerError
			}
		}

		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(allJobsResponse{
			Scope:     "all_jobs",
			Refreshed: refreshed,
			Error:     responseErr,
			Snapshot:  snap,
		}); err != nil {
			log.Warn("failed to encode json response", "error", err)
		}
	}
}

func refreshRequested(r *http.Request) (bool, error) {
	raw := r.URL.Query().Get("refresh")
	if raw == "" {
		raw = r.URL.Query().Get("trigger")
	}
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("refresh must be a boolean")
	}
	return v, nil
}

func queuesHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot().Queues })
}

func hostsHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot().Hosts })
}

func clusterHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot().Cluster })
}

func licensesHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot().Licenses })
}

func resourcesHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return jsonHandler(log, func() any { return svc.Snapshot().CustomResources })
}

func jsonHandler(log *logger.Logger, value func() any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(value()); err != nil {
			log.Warn("failed to encode json response", "error", err)
		}
	}
}

type labels map[string]string

func writeHelp(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, escapeHelp(help))
}

func writeGauge(b *strings.Builder, name string, labels labels, value float64) {
	writeSample(b, name, labels, value)
}

func writeCounter(b *strings.Builder, name string, labels labels, value float64) {
	writeSample(b, name, labels, value)
}

func writeSample(b *strings.Builder, name string, labels labels, value float64) {
	b.WriteString(name)
	if len(labels) > 0 {
		b.WriteByte('{')
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `%s="%s"`, key, escapeLabel(labels[key]))
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(b, " %g\n", value)
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func escapeHelp(value string) string {
	return strings.ReplaceAll(value, "\n", `\n`)
}
