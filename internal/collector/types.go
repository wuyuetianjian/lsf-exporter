package collector

import "time"

type LSFConfig struct {
	AppName      string
	QueryUser    string
	QueryQueue   string
	QueryHost    string
	QueryJobName string
	QueryJobID   int64
	QueryAllJobs bool
}

type ExternalConfig struct {
	Command string
	Timeout time.Duration
}

type ServiceConfig struct {
	Interval    time.Duration
	MinInterval time.Duration
	Timeout     time.Duration
}

type Source interface {
	Collect() (Data, error)
	Close() error
}

type JobQuerySource interface {
	Source
	CollectJobs(includeFinished bool) (Data, error)
}

type Data struct {
	Jobs            []Job            `json:"jobs,omitempty"`
	Queues          []Queue          `json:"queues,omitempty"`
	Hosts           []Host           `json:"hosts,omitempty"`
	Cluster         *Cluster         `json:"cluster,omitempty"`
	Licenses        []LicenseFeature `json:"licenses,omitempty"`
	CustomResources []CustomResource `json:"custom_resources,omitempty"`
}

type Job struct {
	ID            int64             `json:"id"`
	ArrayIndex    int64             `json:"array_index,omitempty"`
	Name          string            `json:"name,omitempty"`
	User          string            `json:"user,omitempty"`
	Queue         string            `json:"queue,omitempty"`
	Status        string            `json:"status"`
	StatusCode    int               `json:"status_code"`
	Project       string            `json:"project,omitempty"`
	Application   string            `json:"application,omitempty"`
	ServiceClass  string            `json:"service_class,omitempty"`
	FromHost      string            `json:"from_host,omitempty"`
	ExecutionHost string            `json:"execution_host,omitempty"`
	Command       string            `json:"command,omitempty"`
	CWD           string            `json:"cwd,omitempty"`
	InputFile     string            `json:"input_file,omitempty"`
	OutputFile    string            `json:"output_file,omitempty"`
	ErrorFile     string            `json:"error_file,omitempty"`
	SubmitTime    int64             `json:"submit_time,omitempty"`
	StartTime     int64             `json:"start_time,omitempty"`
	EndTime       int64             `json:"end_time,omitempty"`
	ExitStatus    int               `json:"exit_status,omitempty"`
	ExitReason    string            `json:"exit_reason,omitempty"`
	PendingReason string            `json:"pending_reason,omitempty"`
	CPUTime       float64           `json:"cpu_time_seconds,omitempty"`
	MemoryKB      int64             `json:"memory_kb,omitempty"`
	SwapKB        int64             `json:"swap_kb,omitempty"`
	GPURequested  float64           `json:"gpu_requested,omitempty"`
	GPUUsed       float64           `json:"gpu_used,omitempty"`
	ResourceReq   string            `json:"resource_requirement,omitempty"`
	Dependency    string            `json:"dependency_condition,omitempty"`
	Raw           map[string]string `json:"raw,omitempty"`
}

type Queue struct {
	Name      string            `json:"name"`
	Status    string            `json:"status,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Open      *bool             `json:"open,omitempty"`
	Active    *bool             `json:"active,omitempty"`
	MaxJobs   int               `json:"max_jobs,omitempty"`
	NumJobs   int               `json:"num_jobs,omitempty"`
	Pending   int               `json:"pending,omitempty"`
	Running   int               `json:"running,omitempty"`
	Suspended int               `json:"suspended,omitempty"`
	Raw       map[string]string `json:"raw,omitempty"`
}

type Host struct {
	Name      string             `json:"name"`
	Status    string             `json:"status,omitempty"`
	Closed    *bool              `json:"closed,omitempty"`
	MaxJobs   int                `json:"max_jobs,omitempty"`
	NumJobs   int                `json:"num_jobs,omitempty"`
	Running   int                `json:"running,omitempty"`
	Suspended int                `json:"suspended,omitempty"`
	Load      map[string]float64 `json:"load,omitempty"`
	Resources map[string]string  `json:"resources,omitempty"`
	Raw       map[string]string  `json:"raw,omitempty"`
}

type Cluster struct {
	Name     string            `json:"name,omitempty"`
	Master   string            `json:"master,omitempty"`
	Status   string            `json:"status,omitempty"`
	MasterUp *bool             `json:"master_up,omitempty"`
	Raw      map[string]string `json:"raw,omitempty"`
}

type LicenseFeature struct {
	Feature string            `json:"feature"`
	Total   float64           `json:"total,omitempty"`
	Used    float64           `json:"used,omitempty"`
	Free    float64           `json:"free,omitempty"`
	Raw     map[string]string `json:"raw,omitempty"`
}

type CustomResource struct {
	Name     string            `json:"name"`
	Type     string            `json:"type,omitempty"`
	Location string            `json:"location,omitempty"`
	Total    float64           `json:"total,omitempty"`
	Used     float64           `json:"used,omitempty"`
	Free     float64           `json:"free,omitempty"`
	Raw      map[string]string `json:"raw,omitempty"`
}

type Snapshot struct {
	Data
	CollectedAt time.Time `json:"collected_at"`
	Duration    string    `json:"duration"`
	Error       string    `json:"error,omitempty"`
}
