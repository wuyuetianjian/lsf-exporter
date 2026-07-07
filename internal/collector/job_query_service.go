package collector

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"lsf-exporter/internal/logger"
)

var ErrJobQueryInProgress = errors.New("job query is already running")
var ErrJobQueryTooSoon = errors.New("job query was triggered too recently")

type JobQueryService struct {
	source JobQuerySource
	cfg    ServiceConfig
	logger *logger.Logger

	running atomic.Bool

	mu       sync.RWMutex
	snapshot Snapshot

	lastAttempt atomic.Int64
}

func NewJobQueryService(source JobQuerySource, cfg ServiceConfig, logger *logger.Logger) *JobQueryService {
	if source == nil {
		return nil
	}
	return &JobQueryService{
		source: source,
		cfg:    cfg,
		logger: logger,
	}
}

func (s *JobQueryService) CollectAllJobs() (Snapshot, error) {
	now := time.Now()
	last := time.Unix(s.lastAttempt.Load(), 0)
	if s.cfg.MinInterval > 0 && !last.IsZero() && now.Sub(last) < s.cfg.MinInterval {
		return s.Snapshot(), ErrJobQueryTooSoon
	}
	if !s.running.CompareAndSwap(false, true) {
		return s.Snapshot(), ErrJobQueryInProgress
	}
	defer s.running.Store(false)

	s.lastAttempt.Store(now.Unix())
	start := time.Now()
	data, err := s.source.CollectJobs(true)
	duration := time.Since(start)

	snap := Snapshot{
		Data:        data,
		CollectedAt: time.Now(),
		Duration:    duration.String(),
	}
	if err != nil {
		snap.Error = err.Error()
		s.logger.Warn("full LSF job query failed", "error", err, "duration", duration.String())
	} else if s.cfg.Timeout > 0 && duration > s.cfg.Timeout {
		s.logger.Warn("full LSF job query exceeded configured timeout threshold", "jobs", len(data.Jobs), "duration", duration.String(), "threshold", s.cfg.Timeout.String())
	} else {
		s.logger.Debug("full LSF job query completed", "jobs", len(data.Jobs), "duration", duration.String())
	}

	s.mu.Lock()
	s.snapshot = snap
	s.mu.Unlock()

	return snap, err
}

func (s *JobQueryService) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]Job, len(s.snapshot.Jobs))
	copy(jobs, s.snapshot.Jobs)
	snap := s.snapshot
	snap.Jobs = jobs
	return snap
}
