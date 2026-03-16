package scheduler

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/titaev-lv/cts-core/internal/api/ws"
)

const sessionStateActive = "active"

const (
	taskTypeTrade   = "trade"
	taskTypeMonitor = "monitor"

	loadWeight                   = 1000.0
	monitorCapacityPenaltyWeight = 200.0
	missingLatencyProfilePenalty = 5000.0
	missingExchangePenalty       = 300.0

	defaultResourceHardLimit     = 0.98
	defaultResourceSoftLimit     = 0.75
	defaultResourceSoftPenaltyMs = 600.0
)

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

// LatencyTestDispatcher pushes server-initiated latency tests to active trader sessions.
type LatencyTestDispatcher interface {
	DispatchLatencyTest(ctx context.Context, sessionID string, traderID string, exchanges []string) error
}

// ExchangeRef describes an exchange in DB-backed requirements sets.
type ExchangeRef struct {
	ExchangeID   int
	ExchangeName string
}

// RequiredExchangesProvider returns DB-backed exchange sets for task scopes.
type RequiredExchangesProvider interface {
	GetTradeRequiredExchanges(ctx context.Context) ([]ExchangeRef, error)
	GetMonitorRequiredExchanges(ctx context.Context) ([]ExchangeRef, error)
}

// TraderResourceProvider reports utilization ratio [0..1] for trader/exchange pair.
type TraderResourceProvider interface {
	GetTraderExchangeUtilization(ctx context.Context, traderDBID int, exchangeID int) (utilization float64, found bool, err error)
}

// Candidate is an eligible trader for assignment cycle.
type Candidate struct {
	TraderID          string
	SessionID         string
	LastHeartbeatUnix int64
	Score             float64
}

// EngineStats contains basic cycle telemetry.
type EngineStats struct {
	Cycles            int64
	LastCandidateSize int64
}

// Config defines scheduler loop behavior.
type Config struct {
	Interval                  time.Duration
	LatencyCheckInterval      time.Duration
	HealthyWindow             time.Duration
	MetricsSink               MetricsSink
	TaskType                  string
	RequiredExchangesProvider RequiredExchangesProvider
	LatencyDispatcher         LatencyTestDispatcher
	ResourceProvider          TraderResourceProvider
	ResourceHardLimit         float64
	ResourceSoftLimit         float64
	ResourceSoftPenaltyMs     float64
}

// Engine runs periodic assignment cycles from runtime session snapshots.
type Engine struct {
	logger        *slog.Logger
	provider      SnapshotProvider
	assignment    AssignmentRunner
	interval      time.Duration
	latencyCheck  time.Duration
	healthyWindow time.Duration
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	cycles        atomic.Int64
	lastCandidate atomic.Int64
	metricsSink   MetricsSink
	taskType      string
	requirements  RequiredExchangesProvider
	dispatcher    LatencyTestDispatcher
	resources     TraderResourceProvider
	resourceHard  float64
	resourceSoft  float64
	resourceW     float64
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

	resourceHard := normalizeResourceHardLimit(cfg.ResourceHardLimit)
	resourceSoft := normalizeResourceSoftLimit(cfg.ResourceSoftLimit)
	if resourceSoft >= resourceHard {
		resourceSoft = resourceHard * 0.8
	}

	return &Engine{
		logger:        logger,
		provider:      provider,
		assignment:    assignment,
		interval:      interval,
		latencyCheck:  cfg.LatencyCheckInterval,
		healthyWindow: healthyWindow,
		metricsSink:   cfg.MetricsSink,
		taskType:      normalizeTaskType(cfg.TaskType),
		requirements:  cfg.RequiredExchangesProvider,
		dispatcher:    cfg.LatencyDispatcher,
		resources:     cfg.ResourceProvider,
		resourceHard:  resourceHard,
		resourceSoft:  resourceSoft,
		resourceW:     normalizeResourceSoftPenalty(cfg.ResourceSoftPenaltyMs),
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
		assignTicker := time.NewTicker(e.interval)
		defer assignTicker.Stop()

		var latencyTicker *time.Ticker
		if e.dispatcher != nil && e.latencyCheck > 0 {
			latencyTicker = time.NewTicker(e.latencyCheck)
			defer latencyTicker.Stop()
		}

		e.runCycle(time.Now().UTC())
		if latencyTicker != nil {
			e.runLatencySweep(time.Now().UTC())
		}
		for {
			select {
			case <-assignTicker.C:
				e.runCycle(time.Now().UTC())
			case <-latencyTickerChan(latencyTicker):
				e.runLatencySweep(time.Now().UTC())
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

// RunLatencySweepForTest executes one latency-test sweep without ticker.
func (e *Engine) RunLatencySweepForTest(now time.Time) {
	e.runLatencySweep(now)
}

func (e *Engine) runCycle(now time.Time) {
	if e.provider == nil {
		return
	}

	snapshots := e.provider.GetTraderSnapshots()
	requiredExchangeRefs := e.resolveRequiredExchangeRefs(context.Background())
	requiredExchanges := namesFromExchangeRefs(requiredExchangeRefs)
	candidates := SelectCandidatesForTask(snapshots, now, e.healthyWindow, e.taskType, requiredExchanges)
	candidates = e.applyResourceConstraints(context.Background(), candidates, snapshots, requiredExchangeRefs)
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

func (e *Engine) resolveRequiredExchangeRefs(ctx context.Context) []ExchangeRef {
	if e.requirements == nil {
		return nil
	}

	var (
		refs []ExchangeRef
		err  error
	)
	switch e.taskType {
	case taskTypeMonitor:
		refs, err = e.requirements.GetMonitorRequiredExchanges(ctx)
	default:
		refs, err = e.requirements.GetTradeRequiredExchanges(ctx)
	}
	if err != nil {
		e.logger.Warn("scheduler_required_exchanges", "task_type", e.taskType, "error", err)
		return nil
	}
	return refs
}

func namesFromExchangeRefs(refs []ExchangeRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.ExchangeName)
	}
	return normalizeExchangeList(names)
}

func (e *Engine) applyResourceConstraints(ctx context.Context, candidates []Candidate, snapshots []ws.TraderSnapshot, required []ExchangeRef) []Candidate {
	if len(candidates) == 0 || e.resources == nil || len(required) == 0 {
		return candidates
	}

	bySession := make(map[string]ws.TraderSnapshot, len(snapshots))
	for _, snap := range snapshots {
		bySession[snap.SessionID] = snap
	}

	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		snap, ok := bySession[candidate.SessionID]
		if !ok {
			continue
		}
		if snap.TraderDBID <= 0 {
			e.logger.Warn("scheduler_resource_reject", "trader_id", candidate.TraderID, "session_id", candidate.SessionID, "reason", "missing_trader_db_id")
			continue
		}

		softPenalty := 0.0
		hardRejected := false
		for _, ref := range required {
			if ref.ExchangeID <= 0 {
				continue
			}

			util, found, err := e.resources.GetTraderExchangeUtilization(ctx, snap.TraderDBID, ref.ExchangeID)
			if err != nil {
				e.logger.Warn("scheduler_resource_reject", "trader_id", candidate.TraderID, "session_id", candidate.SessionID, "exchange_id", ref.ExchangeID, "reason", "resource_lookup_error", "error", err)
				hardRejected = true
				break
			}
			if !found {
				e.logger.Warn("scheduler_resource_reject", "trader_id", candidate.TraderID, "session_id", candidate.SessionID, "exchange_id", ref.ExchangeID, "reason", "missing_resource")
				hardRejected = true
				break
			}

			util = clampUnit(util)
			if util >= e.resourceHard {
				e.logger.Info("scheduler_resource_reject", "trader_id", candidate.TraderID, "session_id", candidate.SessionID, "exchange_id", ref.ExchangeID, "reason", "hard_limit", "utilization", util)
				hardRejected = true
				break
			}

			if util > e.resourceSoft {
				norm := (util - e.resourceSoft) / (1.0 - e.resourceSoft)
				softPenalty += e.resourceW * norm * norm
			}
		}

		if hardRejected {
			continue
		}

		if softPenalty > 0 {
			candidate.Score += softPenalty
		}
		result = append(result, candidate)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score < result[j].Score
		}
		if result[i].LastHeartbeatUnix != result[j].LastHeartbeatUnix {
			return result[i].LastHeartbeatUnix > result[j].LastHeartbeatUnix
		}
		return result[i].TraderID < result[j].TraderID
	})
	return result
}

func (e *Engine) runLatencySweep(now time.Time) {
	if e.provider == nil || e.dispatcher == nil {
		return
	}

	targets := SelectLatencyTestTargets(e.provider.GetTraderSnapshots(), now, e.healthyWindow)
	if len(targets) == 0 {
		e.logger.Debug("scheduler_latency_sweep", "target_count", 0)
		return
	}

	failures := 0
	for _, target := range targets {
		if err := e.dispatcher.DispatchLatencyTest(context.Background(), target.SessionID, target.TraderID, target.EffectiveExchanges); err != nil {
			failures++
			e.logger.Warn("scheduler_latency_dispatch", "trader_id", target.TraderID, "session_id", target.SessionID, "error", err)
		}
	}

	e.logger.Info("scheduler_latency_sweep", "target_count", len(targets), "failed_count", failures)
}

func latencyTickerChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

func (e *Engine) syncMetrics(cycleCount int64, candidateCount int64, lastRunUnix int64) {
	if e.metricsSink == nil {
		return
	}
	e.metricsSink.SetRuntimeScheduler(cycleCount, candidateCount, lastRunUnix)
}

// SelectCandidates keeps only active and healthy WS sessions.
func SelectCandidates(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration) []Candidate {
	return SelectCandidatesByTaskType(snapshots, now, healthyWindow, taskTypeTrade)
}

// SelectCandidatesForTask keeps only active/healthy sessions and applies task-specific
// eligibility + scoring, including required exchanges for trade tasks.
func SelectCandidatesForTask(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration, taskType string, requiredExchanges []string) []Candidate {
	requiredExchanges = normalizeExchangeList(requiredExchanges)
	return selectCandidatesInternal(snapshots, now, healthyWindow, taskType, requiredExchanges)
}

// SelectCandidatesByTaskType keeps only active and healthy WS sessions and
// applies task-type specific eligibility/sorting rules.
func SelectCandidatesByTaskType(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration, taskType string) []Candidate {
	return selectCandidatesInternal(snapshots, now, healthyWindow, taskType, nil)
}

func selectCandidatesInternal(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration, taskType string, requiredExchanges []string) []Candidate {
	if healthyWindow <= 0 {
		healthyWindow = 15 * time.Second
	}
	nowUnix := now.Unix()
	type normalizedCandidate struct {
		candidate   Candidate
		role        string
		load        float64
		tradeLoad   float64
		rolePenalty int
		score       float64
	}

	working := make([]normalizedCandidate, 0, len(snapshots))
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
		role := normalizeRole(s.Role)
		if taskType == taskTypeTrade && role == "monitor" {
			continue
		}

		load := clampUnit(s.LoadIndex)
		tradeLoad := clampUnit(s.TradeLoadIndex)
		if tradeLoad == 0 && load > 0 {
			tradeLoad = load
		}

		item := normalizedCandidate{
			candidate: Candidate{
				TraderID:          s.TraderID,
				SessionID:         s.SessionID,
				LastHeartbeatUnix: s.LastHeartbeatUnix,
			},
			role:        role,
			load:        load,
			tradeLoad:   tradeLoad,
			rolePenalty: monitorRolePenalty(role),
		}

		if taskType == taskTypeMonitor {
			item.score = scoreMonitor(item.tradeLoad, item.load, item.rolePenalty)
		} else {
			item.score = scoreTrade(estimateLatencyProfileMsForTask(s, requiredExchanges), item.load)
		}
		item.candidate.Score = item.score

		working = append(working, item)
	}

	sort.SliceStable(working, func(i, j int) bool {
		left := working[i]
		right := working[j]

		if left.score != right.score {
			return left.score < right.score
		}

		if left.candidate.LastHeartbeatUnix != right.candidate.LastHeartbeatUnix {
			return left.candidate.LastHeartbeatUnix > right.candidate.LastHeartbeatUnix
		}
		return left.candidate.TraderID < right.candidate.TraderID
	})

	result := make([]Candidate, 0, len(working))
	for _, item := range working {
		result = append(result, item.candidate)
	}
	return result
}

// SelectLatencyTestTargets returns active/healthy sessions that can receive periodic latency tests.
func SelectLatencyTestTargets(snapshots []ws.TraderSnapshot, now time.Time, healthyWindow time.Duration) []ws.TraderSnapshot {
	if healthyWindow <= 0 {
		healthyWindow = 15 * time.Second
	}
	nowUnix := now.Unix()
	targets := make([]ws.TraderSnapshot, 0, len(snapshots))
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
		s.EffectiveExchanges = normalizeExchangeList(s.EffectiveExchanges)
		if len(s.EffectiveExchanges) == 0 {
			continue
		}
		targets = append(targets, s)
	}

	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].LastHeartbeatUnix != targets[j].LastHeartbeatUnix {
			return targets[i].LastHeartbeatUnix > targets[j].LastHeartbeatUnix
		}
		return targets[i].TraderID < targets[j].TraderID
	})
	return targets
}

func normalizeTaskType(v string) string {
	switch v {
	case taskTypeMonitor:
		return taskTypeMonitor
	default:
		return taskTypeTrade
	}
}

func normalizeRole(v string) string {
	switch v {
	case "monitor", "trade", "both":
		return v
	default:
		return "both"
	}
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func monitorRolePenalty(role string) int {
	switch role {
	case "monitor":
		return 0
	case "both":
		return 100
	case "trade":
		return 300
	default:
		return 100
	}
}

func normalizeExchangeList(exchanges []string) []string {
	if len(exchanges) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(exchanges))
	items := make([]string, 0, len(exchanges))
	for _, ex := range exchanges {
		norm := strings.ToLower(strings.TrimSpace(ex))
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		items = append(items, norm)
	}
	return items
}

func normalizeResourceHardLimit(v float64) float64 {
	if v <= 0 || v >= 1 {
		return defaultResourceHardLimit
	}
	return v
}

func normalizeResourceSoftLimit(v float64) float64 {
	if v <= 0 || v >= 1 {
		return defaultResourceSoftLimit
	}
	return v
}

func normalizeResourceSoftPenalty(v float64) float64 {
	if v <= 0 {
		return defaultResourceSoftPenaltyMs
	}
	return v
}

func scoreTrade(latencyProfileMs float64, loadIndex float64) float64 {
	return latencyProfileMs + loadWeight*(loadIndex*loadIndex)
}

func scoreMonitor(tradeLoadIndex float64, loadIndex float64, rolePenalty int) float64 {
	monitorCapacityPenalty := monitorCapacityPenaltyWeight * (loadIndex * loadIndex)
	return loadWeight*(tradeLoadIndex*tradeLoadIndex) + monitorCapacityPenalty + float64(rolePenalty)
}

func estimateLatencyProfileMs(snapshot ws.TraderSnapshot) float64 {
	if snapshot.LatencyProfileMs > 0 {
		return snapshot.LatencyProfileMs
	}
	if len(snapshot.EffectiveExchanges) == 0 {
		return missingLatencyProfilePenalty
	}
	return missingLatencyProfilePenalty
}

func estimateLatencyProfileMsForTask(snapshot ws.TraderSnapshot, requiredExchanges []string) float64 {
	if len(requiredExchanges) == 0 {
		return estimateLatencyProfileMs(snapshot)
	}
	if len(snapshot.ExchangeLatencies) == 0 {
		return missingLatencyProfilePenalty + float64(len(requiredExchanges))*missingExchangePenalty
	}

	latencies := make([]float64, 0, len(requiredExchanges))
	missingCount := 0
	for _, ex := range requiredExchanges {
		latency, ok := snapshot.ExchangeLatencies[ex]
		if !ok {
			missingCount++
			continue
		}
		if latency < 0 {
			missingCount++
			continue
		}
		latencies = append(latencies, latency)
	}

	if len(latencies) == 0 {
		return missingLatencyProfilePenalty + float64(len(requiredExchanges))*missingExchangePenalty
	}

	sort.Float64s(latencies)
	maxLatency := latencies[len(latencies)-1]
	idx95 := int(math.Ceil(0.95*float64(len(latencies)))) - 1
	if idx95 < 0 {
		idx95 = 0
	}
	if idx95 >= len(latencies) {
		idx95 = len(latencies) - 1
	}
	p95 := latencies[idx95]
	spread := maxLatency - latencies[0]
	profile := p95 + 0.10*spread
	if missingCount > 0 {
		profile += float64(missingCount) * missingExchangePenalty
	}
	return profile
}
