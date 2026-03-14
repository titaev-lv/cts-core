package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/titaev-lv/cts-core/internal/api/ws"
)

const sessionStateActive = "active"

// SnapshotProvider returns current runtime WS trader snapshots.
type SnapshotProvider interface {
	GetTraderSnapshots() []ws.TraderSnapshot
}

// AssignmentRunner executes placeholder assignment operation.
type AssignmentRunner interface {
	Assign(ctx context.Context, candidate Candidate) error
}

// MetricsSink receives runtime scheduler telemetry for health/state surfaces.
type MetricsSink interface {
	SetRuntimeScheduler(cycleCount int64, lastCandidateCount int64, lastRunUnix int64)
}

// Candidate is an eligible trader for assignment cycle.
type Candidate struct {
	TraderID          string
	SessionID         string
	LastHeartbeatUnix int64
}

// EngineStats contains basic cycle telemetry.
type EngineStats struct {
	Cycles            int64
	LastCandidateSize int64
}

// Config defines scheduler loop behavior.
type Config struct {
	Interval      time.Duration
	HealthyWindow time.Duration
	MetricsSink   MetricsSink
}

// Engine runs periodic assignment cycles from runtime session snapshots.
type Engine struct {
	logger        *slog.Logger
	provider      SnapshotProvider
	assignment    AssignmentRunner
	interval      time.Duration
	healthyWindow time.Duration
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	cycles        atomic.Int64
	lastCandidate atomic.Int64
	metricsSink   MetricsSink
}

// NoopAssignment is a placeholder assignment implementation.
type NoopAssignment struct{}

func (NoopAssignment) Assign(_ context.Context, _ Candidate) error {
	return nil
}

func NewEngine(cfg Config, provider SnapshotProvider, assignment AssignmentRunner, logger *slog.Logger) *Engine {
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Second
	}

	healthyWindow := cfg.HealthyWindow
	if healthyWindow <= 0 {
		healthyWindow = 15 * time.Second
	}

	if logger == nil {
		logger = slog.Default()
	}
	if assignment == nil {
		assignment = NoopAssignment{}
	}

	return &Engine{
		logger:        logger,
		provider:      provider,
		assignment:    assignment,
		interval:      interval,
		healthyWindow: healthyWindow,
		metricsSink:   cfg.MetricsSink,
		stopCh:        make(chan struct{}),
	}
}

func (e *Engine) Start() {
	if e.provider == nil {
		e.logger.Warn("scheduler_disabled", "reason", "missing_snapshot_provider")
		return
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		e.runCycle(time.Now().UTC())
		for {
			select {
			case <-ticker.C:
				e.runCycle(time.Now().UTC())
			case <-e.stopCh:
				return
			}
		}
	}()
}

func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.wg.Wait()
	})
}

func (e *Engine) Stats() EngineStats {
	return EngineStats{
		Cycles:            e.cycles.Load(),
		LastCandidateSize: e.lastCandidate.Load(),
	}
}

// RunCycleForTest executes one cycle without ticker.
func (e *Engine) RunCycleForTest(now time.Time) {
	e.runCycle(now)
}

func (e *Engine) runCycle(now time.Time) {
	if e.provider == nil {
		return
	}

	snapshots := e.provider.GetTraderSnapshots()
	candidates := SelectCandidates(snapshots, now, e.healthyWindow)
	e.lastCandidate.Store(int64(len(candidates)))
	cycle := e.cycles.Add(1)
	e.syncMetrics(cycle, int64(len(candidates)), now.Unix())

	if len(candidates) == 0 {
		e.logger.Debug("scheduler_cycle", "cycle", cycle, "snapshot_count", len(snapshots), "candidate_count", 0, "assigned", false)
		return
	}

	selected := candidates[0]
	err := e.assignment.Assign(context.Background(), selected)
	if err != nil {
		e.logger.Warn("scheduler_cycle", "cycle", cycle, "snapshot_count", len(snapshots), "candidate_count", len(candidates), "assigned", false, "trader_id", selected.TraderID, "session_id", selected.SessionID, "error", err)
		return
	}

	e.logger.Info("scheduler_cycle", "cycle", cycle, "snapshot_count", len(snapshots), "candidate_count", len(candidates), "assigned", true, "trader_id", selected.TraderID, "session_id", selected.SessionID)
}

func (e *Engine) syncMetrics(cycleCount int64, candidateCount int64, lastRunUnix int64) {
	if e.metricsSink == nil {
		return
	}
	e.metricsSink.SetRuntimeScheduler(cycleCount, candidateCount, lastRunUnix)
}

// SelectCandidates keeps only active and healthy WS sessions.
func SelectCandidates(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration) []Candidate {
	if healthyWindow <= 0 {
		healthyWindow = 15 * time.Second
	}
	nowUnix := now.Unix()

	result := make([]Candidate, 0, len(snapshots))
	for _, s := range snapshots {
		if s.TraderID == "" || s.SessionID == "" {
			continue
		}
		if s.State != sessionStateActive {
			continue
		}
		if s.LastHeartbeatUnix <= 0 {
			continue
		}
		if nowUnix-s.LastHeartbeatUnix > int64(healthyWindow.Seconds()) {
			continue
		}

		result = append(result, Candidate{
			TraderID:          s.TraderID,
			SessionID:         s.SessionID,
			LastHeartbeatUnix: s.LastHeartbeatUnix,
		})
	}
	return result
}
