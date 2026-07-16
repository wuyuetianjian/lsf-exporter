//go:build lsf

package collector

/*
#cgo LDFLAGS: -lbat -llsf
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <lsf/lsf.h>
#include <lsf/lsbatch.h>

#ifndef CUR_JOB
#define CUR_JOB 0
#endif

#ifndef ALL_JOB
#define ALL_JOB CUR_JOB
#endif

typedef struct {
	long long id;
	long long array_index;
	int status_code;
	char *status;
	char *user;
	char *queue;
	char *name;
	char *project;
	char *application;
	char *service_class;
	char *from_host;
	char *execution_host;
	char *command;
	char *cwd;
	char *input_file;
	char *output_file;
	char *error_file;
	char *resource_requirement;
	char *dependency_condition;
	long long submit_time;
	long long start_time;
	long long end_time;
	int exit_status;
	int requested_cpu;
	double cpu_time;
	long long memory_kb;
	long long swap_kb;
} lsf_exporter_job;

static char *dupstr(const char *s) {
	if (s == NULL) {
		return NULL;
	}
	return strdup(s);
}

static char *join_hosts(char **hosts, int n) {
	if (hosts == NULL || n <= 0) {
		return NULL;
	}
	size_t total = 1;
	for (int i = 0; i < n; i++) {
		if (hosts[i] != NULL) {
			total += strlen(hosts[i]) + 1;
		}
	}
	char *out = (char *)calloc(total, sizeof(char));
	if (out == NULL) {
		return NULL;
	}
	for (int i = 0; i < n; i++) {
		if (hosts[i] == NULL) {
			continue;
		}
		if (out[0] != '\0') {
			strcat(out, ",");
		}
		strcat(out, hosts[i]);
	}
	return out;
}

static char *status_text(int status) {
	if (status & JOB_STAT_PEND) return dupstr("PEND");
	if (status & JOB_STAT_RUN) return dupstr("RUN");
	if (status & JOB_STAT_PSUSP) return dupstr("PSUSP");
	if (status & JOB_STAT_USUSP) return dupstr("USUSP");
	if (status & JOB_STAT_SSUSP) return dupstr("SSUSP");
	if (status & JOB_STAT_DONE) return dupstr("DONE");
	if (status & JOB_STAT_EXIT) return dupstr("EXIT");
	if (status & JOB_STAT_UNKWN) return dupstr("UNKWN");
	if (status & JOB_STAT_WAIT) return dupstr("WAIT");
	return dupstr("UNKNOWN");
}

static void set_error(char *err, int err_len, const char *prefix) {
	if (err == NULL || err_len <= 0) {
		return;
	}
	const char *msg = lsb_sysmsg();
	if (msg == NULL) {
		msg = "unknown LSF error";
	}
	snprintf(err, err_len, "%s: %s", prefix, msg);
}

static int lsf_exporter_init(char *app_name, char *err, int err_len) {
	if (lsb_init(app_name) < 0) {
		set_error(err, err_len, "lsb_init failed");
		return -1;
	}
	return 0;
}

static void lsf_exporter_free_job(lsf_exporter_job *job);
static void lsf_exporter_free_jobs(lsf_exporter_job *jobs, int count);

static void fill_job(struct jobInfoEnt *job, lsf_exporter_job *out) {
	memset(out, 0, sizeof(lsf_exporter_job));
	out->id = (long long)job->jobId;
	out->status_code = job->status;
	out->status = status_text(job->status);
	out->user = dupstr(job->user);
	out->queue = dupstr(job->submit.queue);
	out->name = dupstr(job->submit.jobName);
	out->project = dupstr(job->submit.projectName);
	out->application = dupstr(job->submit.app);
	out->service_class = dupstr(job->submit.sla);
	out->from_host = dupstr(job->fromHost);
	out->execution_host = join_hosts(job->exHosts, job->numExHosts);
	out->command = dupstr(job->submit.command);
	out->cwd = dupstr(job->cwd);
	if (out->cwd == NULL) {
		out->cwd = dupstr(job->submit.cwd);
	}
	out->input_file = dupstr(job->submit.inFile);
	out->output_file = dupstr(job->submit.outFile);
	out->error_file = dupstr(job->submit.errFile);
	out->resource_requirement = dupstr(job->submit.resReq);
	out->dependency_condition = dupstr(job->submit.dependCond);
	out->submit_time = (long long)job->submitTime;
	out->start_time = (long long)job->startTime;
	out->end_time = (long long)job->endTime;
	out->exit_status = job->exitStatus;
	out->requested_cpu = job->submit.numProcessors;
	out->cpu_time = (double)job->cpuTime;
	out->memory_kb = (long long)job->runRusage.mem;
	out->swap_kb = (long long)job->runRusage.swap;
}

static int lsf_exporter_collect(
	long long job_id,
	char *job_name,
	char *user_name,
	char *queue_name,
	char *host_name,
	int include_finished,
	lsf_exporter_job **out_jobs,
	int *out_count,
	char *err,
	int err_len
) {
	*out_jobs = NULL;
	*out_count = 0;

	int options = include_finished ? ALL_JOB : CUR_JOB;
	int count = lsb_openjobinfo((LS_LONG_INT)job_id, job_name, user_name, queue_name, host_name, options);
	if (count < 0) {
		set_error(err, err_len, "lsb_openjobinfo failed");
		return -1;
	}
	if (count == 0) {
		lsb_closejobinfo();
		return 0;
	}

	lsf_exporter_job *jobs = (lsf_exporter_job *)calloc((size_t)count, sizeof(lsf_exporter_job));
	if (jobs == NULL) {
		lsb_closejobinfo();
		snprintf(err, err_len, "calloc failed for %d jobs", count);
		return -1;
	}

	int more = 0;
	int seen = 0;
	for (int i = 0; i < count; i++) {
		struct jobInfoEnt *job = lsb_readjobinfo(&more);
		if (job == NULL) {
			lsf_exporter_free_jobs(jobs, seen);
			lsb_closejobinfo();
			set_error(err, err_len, "lsb_readjobinfo failed");
			return -1;
		}
		fill_job(job, &jobs[seen]);
		seen++;
		if (more <= 0) {
			break;
		}
	}

	lsb_closejobinfo();
	*out_jobs = jobs;
	*out_count = seen;
	return 0;
}

static void lsf_exporter_free_job(lsf_exporter_job *job) {
	if (job == NULL) {
		return;
	}
	free(job->status);
	free(job->user);
	free(job->queue);
	free(job->name);
	free(job->project);
	free(job->application);
	free(job->service_class);
	free(job->from_host);
	free(job->execution_host);
	free(job->command);
	free(job->cwd);
	free(job->input_file);
	free(job->output_file);
	free(job->error_file);
	free(job->resource_requirement);
	free(job->dependency_condition);
}

static void lsf_exporter_free_jobs(lsf_exporter_job *jobs, int count) {
	if (jobs == NULL) {
		return;
	}
	for (int i = 0; i < count; i++) {
		lsf_exporter_free_job(&jobs[i]);
	}
	free(jobs);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

type lsfSource struct {
	cfg LSFConfig
}

func NewLSFSource(cfg LSFConfig) (Source, error) {
	if cfg.AppName == "" {
		cfg.AppName = "lsf-exporter"
	}
	app := C.CString(cfg.AppName)
	defer C.free(unsafe.Pointer(app))

	errBuf := (*C.char)(C.calloc(1, 4096))
	defer C.free(unsafe.Pointer(errBuf))

	if rc := C.lsf_exporter_init(app, errBuf, 4096); rc != 0 {
		return nil, errors.New(C.GoString(errBuf))
	}
	return &lsfSource{cfg: cfg}, nil
}

func (s *lsfSource) Collect() (Data, error) {
	return s.collect(s.cfg)
}

func (s *lsfSource) CollectJobs(includeFinished bool) (Data, error) {
	cfg := s.cfg
	cfg.QueryAllJobs = includeFinished
	return s.collect(cfg)
}

func (s *lsfSource) collect(cfg LSFConfig) (Data, error) {
	var cJobs *C.lsf_exporter_job
	var cCount C.int
	errBuf := (*C.char)(C.calloc(1, 4096))
	defer C.free(unsafe.Pointer(errBuf))

	jobName := cStringOrNil(cfg.QueryJobName)
	userName := cStringOrNil(cfg.QueryUser)
	queueName := cStringOrNil(cfg.QueryQueue)
	hostName := cStringOrNil(cfg.QueryHost)
	defer freeCString(jobName)
	defer freeCString(userName)
	defer freeCString(queueName)
	defer freeCString(hostName)

	includeFinished := C.int(0)
	if cfg.QueryAllJobs {
		includeFinished = 1
	}

	rc := C.lsf_exporter_collect(
		C.longlong(cfg.QueryJobID),
		jobName,
		userName,
		queueName,
		hostName,
		includeFinished,
		&cJobs,
		&cCount,
		errBuf,
		4096,
	)
	if rc != 0 {
		return Data{}, errors.New(C.GoString(errBuf))
	}
	defer C.lsf_exporter_free_jobs(cJobs, cCount)

	records := unsafe.Slice(cJobs, int(cCount))
	jobs := make([]Job, 0, int(cCount))
	for _, record := range records {
		job := Job{
			ID:            int64(record.id),
			ArrayIndex:    int64(record.array_index),
			StatusCode:    int(record.status_code),
			Status:        C.GoString(record.status),
			User:          C.GoString(record.user),
			Queue:         C.GoString(record.queue),
			Name:          C.GoString(record.name),
			Project:       C.GoString(record.project),
			Application:   C.GoString(record.application),
			ServiceClass:  C.GoString(record.service_class),
			FromHost:      C.GoString(record.from_host),
			ExecutionHost: C.GoString(record.execution_host),
			Command:       C.GoString(record.command),
			CWD:           C.GoString(record.cwd),
			InputFile:     C.GoString(record.input_file),
			OutputFile:    C.GoString(record.output_file),
			ErrorFile:     C.GoString(record.error_file),
			SubmitTime:    int64(record.submit_time),
			StartTime:     int64(record.start_time),
			EndTime:       int64(record.end_time),
			ExitStatus:    int(record.exit_status),
			RequestedCPU:  int(record.requested_cpu),
			CPUTime:       float64(record.cpu_time),
			MemoryKB:      int64(record.memory_kb),
			SwapKB:        int64(record.swap_kb),
			ResourceReq:   C.GoString(record.resource_requirement),
			Dependency:    C.GoString(record.dependency_condition),
		}
		job.RequestedMemKB = requestedMemoryKB(job.ResourceReq)
		job.Raw = map[string]string{
			"job_id":               fmt.Sprintf("%d", job.ID),
			"status_code":          fmt.Sprintf("%d", job.StatusCode),
			"status":               job.Status,
			"user":                 job.User,
			"queue":                job.Queue,
			"name":                 job.Name,
			"project":              job.Project,
			"application":          job.Application,
			"service_class":        job.ServiceClass,
			"from_host":            job.FromHost,
			"execution_host":       job.ExecutionHost,
			"command":              job.Command,
			"cwd":                  job.CWD,
			"input_file":           job.InputFile,
			"output_file":          job.OutputFile,
			"error_file":           job.ErrorFile,
			"submit_time":          fmt.Sprintf("%d", job.SubmitTime),
			"start_time":           fmt.Sprintf("%d", job.StartTime),
			"end_time":             fmt.Sprintf("%d", job.EndTime),
			"exit_status":          fmt.Sprintf("%d", job.ExitStatus),
			"requested_cpu":        fmt.Sprintf("%d", job.RequestedCPU),
			"requested_memory_kb":  fmt.Sprintf("%d", job.RequestedMemKB),
			"cpu_time":             fmt.Sprintf("%f", job.CPUTime),
			"memory_kb":            fmt.Sprintf("%d", job.MemoryKB),
			"swap_kb":              fmt.Sprintf("%d", job.SwapKB),
			"resource_requirement": job.ResourceReq,
			"dependency_condition": job.Dependency,
		}
		jobs = append(jobs, job)
	}
	return Data{Jobs: jobs}, nil
}

func (s *lsfSource) Close() error {
	return nil
}

func cStringOrNil(value string) *C.char {
	if value == "" {
		return nil
	}
	return C.CString(value)
}

func freeCString(value *C.char) {
	if value != nil {
		C.free(unsafe.Pointer(value))
	}
}
