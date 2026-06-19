package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"lsf-exporter/internal/collector"
	"lsf-exporter/internal/logger"
)

func Register(mux *http.ServeMux, svc *collector.Service, log *logger.Logger) {
	mux.HandleFunc("/metrics", metricsHandler(svc))
	mux.HandleFunc("/jobs", jobsHandler(svc, log))
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
		if !stats.LastSuccess.IsZero() {
			writeGauge(&b, "lsf_exporter_last_success_timestamp_seconds", nil, float64(stats.LastSuccess.Unix()))
		}
		if !snap.CollectedAt.IsZero() {
			writeGauge(&b, "lsf_exporter_snapshot_age_seconds", nil, time.Since(snap.CollectedAt).Seconds())
		}
		if d, err := time.ParseDuration(snap.Duration); err == nil {
			writeGauge(&b, "lsf_exporter_last_collect_duration_seconds", nil, d.Seconds())
		}

		byStatus := map[string]int{}
		now := time.Now().Unix()
		for _, job := range snap.Jobs {
			jobID := strconv.FormatInt(job.ID, 10)
			byStatus[job.Status]++
			writeGauge(&b, "lsf_job_info", labels{
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
				writeGauge(&b, "lsf_job_cpu_time_seconds", labels{"job_id": jobID, "status": job.Status}, job.CPUTime)
			}
			if job.MemoryKB > 0 {
				writeGauge(&b, "lsf_job_memory_kilobytes", labels{"job_id": jobID, "status": job.Status}, float64(job.MemoryKB))
			}
			if job.SwapKB > 0 {
				writeGauge(&b, "lsf_job_swap_kilobytes", labels{"job_id": jobID, "status": job.Status}, float64(job.SwapKB))
			}
			if job.StartTime > 0 {
				end := job.EndTime
				if end <= 0 {
					end = now
				}
				if end > job.StartTime {
					writeGauge(&b, "lsf_job_runtime_seconds", labels{"job_id": jobID, "status": job.Status}, float64(end-job.StartTime))
				}
			}
		}
		for status, count := range byStatus {
			writeGauge(&b, "lsf_jobs_total", labels{"status": status}, float64(count))
		}

		_, _ = w.Write([]byte(b.String()))
	}
}

func jobsHandler(svc *collector.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(svc.Snapshot()); err != nil {
			log.Warn("failed to encode jobs response", "error", err)
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

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func escapeHelp(value string) string {
	return strings.ReplaceAll(value, "\n", `\n`)
}
