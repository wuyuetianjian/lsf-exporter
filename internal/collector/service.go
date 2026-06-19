package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"lsf-exporter/internal/logger"
)

type Service struct {
	source Source
	cfg    ServiceConfig
	logger *logger.Logger

	running atomic.Bool

	mu       sync.RWMutex
	snapshot Snapshot

	collections atomic.Uint64
	errors      atomic.Uint64
	skipped     atomic.Uint64
	lastSuccess atomic.Int64
	lastAttempt atomic.Int64
}

func NewService(source Source, cfg ServiceConfig, logger *logger.Logger) *Service {
	return &Service{
		source: source,
		cfg:    cfg,
		logger: logger,
	}
}

func (s *Service) Run(ctx context.Context) {
	s.collectOnce()
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collectOnce()
		}
	}
}

func (s *Service) CollectNowForTest() {
	s.collectOnce()
}

func (s *Service) collectOnce() {
	now := time.Now()
	last := time.Unix(s.lastAttempt.Load(), 0)
	if !last.IsZero() && now.Sub(last) < s.cfg.MinInterval {
		s.skipped.Add(1)
		return
	}
	if !s.running.CompareAndSwap(false, true) {
		s.skipped.Add(1)
		return
	}
	defer s.running.Store(false)

	s.lastAttempt.Store(now.Unix())
	start := time.Now()
	jobs, err := s.source.Collect()
	duration := time.Since(start)
	s.collections.Add(1)

	snap := Snapshot{
		Jobs:        jobs,
		CollectedAt: time.Now(),
		Duration:    duration.String(),
	}
	if err != nil {
		s.errors.Add(1)
		snap.Error = err.Error()
		s.logger.Warn("LSF collection failed", "error", err, "duration", duration.String())
	} else {
		s.lastSuccess.Store(snap.CollectedAt.Unix())
		if s.cfg.Timeout > 0 && duration > s.cfg.Timeout {
			s.logger.Warn("LSF collection exceeded configured timeout threshold", "jobs", len(jobs), "duration", duration.String(), "threshold", s.cfg.Timeout.String())
		} else {
			s.logger.Debug("LSF collection completed", "jobs", len(jobs), "duration", duration.String())
		}
	}

	s.mu.Lock()
	s.snapshot = snap
	s.mu.Unlock()
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]Job, len(s.snapshot.Jobs))
	copy(jobs, s.snapshot.Jobs)
	snap := s.snapshot
	snap.Jobs = jobs
	return snap
}

func (s *Service) Stats() Stats {
	lastSuccessUnix := s.lastSuccess.Load()
	var lastSuccess time.Time
	if lastSuccessUnix > 0 {
		lastSuccess = time.Unix(lastSuccessUnix, 0)
	}
	return Stats{
		Collections: s.collections.Load(),
		Errors:      s.errors.Load(),
		Skipped:     s.skipped.Load(),
		LastSuccess: lastSuccess,
	}
}

type Stats struct {
	Collections uint64
	Errors      uint64
	Skipped     uint64
	LastSuccess time.Time
}
