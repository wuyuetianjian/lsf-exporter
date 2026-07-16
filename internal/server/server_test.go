package server

import (
	"strings"
	"testing"
	"time"

	"lsf-exporter/internal/collector"
)

func TestWriteJobMetricsIncludesRequestedCPUAndCPUTime(t *testing.T) {
	snap := collector.Snapshot{
		Data: collector.Data{
			Jobs: []collector.Job{
				{
					ID:             123,
					Status:         "RUN",
					User:           "alice",
					Queue:          "normal",
					Name:           "cpu-job",
					RequestedCPU:   8,
					RequestedMemKB: 2097152,
					CPUTime:        42.5,
					MemoryKB:       1024000,
					SwapKB:         512000,
					StartTime:      time.Now().Unix() - 10,
				},
			},
		},
	}

	var b strings.Builder
	writeJobMetrics(&b, snap)
	got := b.String()

	if !strings.Contains(got, `lsf_job_requested_cpu{job_id="123",status="RUN"} 8`) {
		t.Fatalf("missing requested CPU metric in:\n%s", got)
	}
	if !strings.Contains(got, `lsf_job_cpu_time_seconds{job_id="123",status="RUN"} 42.5`) {
		t.Fatalf("missing CPU time metric in:\n%s", got)
	}
	if !strings.Contains(got, `lsf_job_requested_memory_kilobytes{job_id="123",status="RUN"} 2.097152e+06`) {
		t.Fatalf("missing requested memory metric in:\n%s", got)
	}
	if !strings.Contains(got, `lsf_job_memory_kilobytes{job_id="123",status="RUN"} 1.024e+06`) {
		t.Fatalf("missing used memory metric in:\n%s", got)
	}
	if !strings.Contains(got, `lsf_job_swap_kilobytes{job_id="123",status="RUN"} 512000`) {
		t.Fatalf("missing used swap metric in:\n%s", got)
	}
}
