package collector

import (
	"errors"
	"strings"
)

type compositeSource struct {
	sources []Source
}

func NewCompositeSource(sources ...Source) Source {
	filtered := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filtered = append(filtered, source)
		}
	}
	return &compositeSource{sources: filtered}
}

func (s *compositeSource) Collect() (Data, error) {
	var out Data
	var errs []error
	for _, source := range s.sources {
		data, err := source.Collect()
		if err != nil {
			errs = append(errs, err)
		}
		mergeData(&out, data)
	}
	return out, errors.Join(errs...)
}

func (s *compositeSource) Close() error {
	var errs []error
	for _, source := range s.sources {
		if err := source.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func mergeData(dst *Data, src Data) {
	dst.Jobs = append(dst.Jobs, src.Jobs...)
	dst.Queues = append(dst.Queues, src.Queues...)
	dst.Hosts = append(dst.Hosts, src.Hosts...)
	dst.Licenses = append(dst.Licenses, src.Licenses...)
	dst.CustomResources = append(dst.CustomResources, src.CustomResources...)
	if src.Cluster != nil {
		cluster := *src.Cluster
		dst.Cluster = &cluster
	}
}

func IsNoopExternalConfig(cfg ExternalConfig) bool {
	return strings.TrimSpace(cfg.Command) == ""
}
