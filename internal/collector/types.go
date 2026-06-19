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

type ServiceConfig struct {
	Interval    time.Duration
	MinInterval time.Duration
	Timeout     time.Duration
}

type Source interface {
	Collect() ([]Job, error)
	Close() error
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
	CPUTime       float64           `json:"cpu_time_seconds,omitempty"`
	MemoryKB      int64             `json:"memory_kb,omitempty"`
	SwapKB        int64             `json:"swap_kb,omitempty"`
	Raw           map[string]string `json:"raw,omitempty"`
}

type Snapshot struct {
	Jobs        []Job     `json:"jobs"`
	CollectedAt time.Time `json:"collected_at"`
	Duration    string    `json:"duration"`
	Error       string    `json:"error,omitempty"`
}
