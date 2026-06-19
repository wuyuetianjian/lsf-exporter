//go:build !lsf

package collector

import "fmt"

func NewLSFSource(cfg LSFConfig) (Source, error) {
	return &stubSource{cfg: cfg}, nil
}

type stubSource struct {
	cfg LSFConfig
}

func (s *stubSource) Collect() ([]Job, error) {
	return nil, fmt.Errorf("LSF C API collector is disabled; rebuild with: go build -tags lsf ./cmd/lsf-exporter")
}

func (s *stubSource) Close() error {
	return nil
}
