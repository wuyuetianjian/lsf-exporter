package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type externalSource struct {
	cfg ExternalConfig
}

func NewExternalSource(cfg ExternalConfig) Source {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &externalSource{cfg: cfg}
}

func (s *externalSource) Collect() (Data, error) {
	command := strings.TrimSpace(s.cfg.Command)
	if command == "" {
		return Data{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Timeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return Data{}, fmt.Errorf("external LSF resource command failed: %w: %s", err, msg)
		}
		return Data{}, fmt.Errorf("external LSF resource command failed: %w", err)
	}

	var data Data
	dec := json.NewDecoder(&stdout)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&data); err != nil {
		return Data{}, fmt.Errorf("external LSF resource command returned invalid JSON: %w", err)
	}
	return data, nil
}

func (s *externalSource) Close() error {
	return nil
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
