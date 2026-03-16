package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/titaev-lv/cts-core/internal/api/ws"
)

type staticProvider struct {
	snapshots []ws.TraderSnapshot
}

func (p staticProvider) GetTraderSnapshots() []ws.TraderSnapshot {
	return p.snapshots
}

type mutableProvider struct {
	mu        sync.RWMutex
	snapshots []ws.TraderSnapshot
}

func (p *mutableProvider) GetTraderSnapshots() []ws.TraderSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := make([]ws.TraderSnapshot, len(p.snapshots))
	copy(items, p.snapshots)
	return items
}

func (p *mutableProvider) setSnapshots(s []ws.TraderSnapshot) {
	p.mu.Lock()
	p.snapshots = s
	p.mu.Unlock()
}

type countingAssignment struct {
	count atomic.Int64
}

type recordingMetricsSink struct {
	cycleCount     atomic.Int64
	candidateCount atomic.Int64
	lastRunUnix    atomic.Int64
}

type latencyDispatchCall struct {
	sessionID string
	traderID  string
	exchanges []string
}

type recordingLatencyDispatcher struct {
	mu    sync.Mutex
	calls []latencyDispatchCall
}

func (d *recordingLatencyDispatcher) DispatchLatencyTest(_ context.Context, sessionID string, traderID string, exchanges []string) error {
	d.mu.Lock()
	d.calls = append(d.calls, latencyDispatchCall{sessionID: sessionID, traderID: traderID, exchanges: append([]string(nil), exchanges...)})
	d.mu.Unlock()
	return nil
}

func (d *recordingLatencyDispatcher) Calls() []latencyDispatchCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]latencyDispatchCall, len(d.calls))
	copy(out, d.calls)
	return out
}

type staticRequirementsProvider struct {
	trade   []ExchangeRef
	monitor []ExchangeRef
	err     error
}

func (p staticRequirementsProvider) GetTradeRequiredExchanges(_ context.Context) ([]ExchangeRef, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.trade, nil
}

func (p staticRequirementsProvider) GetMonitorRequiredExchanges(_ context.Context) ([]ExchangeRef, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.monitor, nil
}

func (s *recordingMetricsSink) SetRuntimeScheduler(cycleCount int64, lastCandidateCount int64, lastRunUnix int64) {
	s.cycleCount.Store(cycleCount)
	s.candidateCount.Store(lastCandidateCount)
	s.lastRunUnix.Store(lastRunUnix)
}

func (a *countingAssignment) Assign(_ context.Context, _ Candidate) error {
	a.count.Add(1)
	return nil
}

func TestSelectCandidates_ActiveAndHealthyOnly(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	candidates := SelectCandidates([]ws.TraderSnapshot{
		{TraderID: "t-1", SessionID: "s-1", State: "active", LastHeartbeatUnix: now.Unix() - 2},
		{TraderID: "t-2", SessionID: "s-2", State: "active", LastHeartbeatUnix: now.Unix() - 20},
		{TraderID: "t-3", SessionID: "s-3", State: "registered", LastHeartbeatUnix: now.Unix() - 1},
		{TraderID: "", SessionID: "s-4", State: "active", LastHeartbeatUnix: now.Unix() - 1},
	}, now, 5*time.Second)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-1" {
		t.Fatalf("expected candidate t-1, got %s", candidates[0].TraderID)
	}
}

func TestRunCycle_NoCandidates_NoAssignment(t *testing.T) {
	assignment := &countingAssignment{}
	engine := NewEngine(Config{Interval: time.Second, HealthyWindow: 5 * time.Second}, staticProvider{}, assignment, slog.Default())
	engine.RunCycleForTest(time.Now().UTC())

	stats := engine.Stats()
	if stats.Cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", stats.Cycles)
	}
	if stats.LastCandidateSize != 0 {
		t.Fatalf("expected 0 candidates, got %d", stats.LastCandidateSize)
	}
	if assignment.count.Load() != 0 {
		t.Fatalf("expected no assignments, got %d", assignment.count.Load())
	}
}

func TestSchedulerLoop_StableUnderRapidChanges(t *testing.T) {
	provider := &mutableProvider{}
	assignment := &countingAssignment{}
	engine := NewEngine(Config{Interval: 5 * time.Millisecond, HealthyWindow: 50 * time.Millisecond}, provider, assignment, slog.Default())

	engine.Start()
	defer engine.Stop()

	start := time.Now().UTC().Unix()
	for i := 0; i < 50; i++ {
		state := "active"
		if i%3 == 0 {
			state = "registered"
		}
		provider.setSnapshots([]ws.TraderSnapshot{
			{TraderID: "t-rapid", SessionID: "s-rapid", State: state, LastHeartbeatUnix: start},
		})
		time.Sleep(2 * time.Millisecond)
	}

	time.Sleep(30 * time.Millisecond)
	stats := engine.Stats()
	if stats.Cycles == 0 {
		t.Fatal("expected scheduler to execute at least one cycle")
	}
}

func TestRunCycle_PublishesMetricsToSink(t *testing.T) {
	metrics := &recordingMetricsSink{}
	engine := NewEngine(
		Config{Interval: time.Second, HealthyWindow: 5 * time.Second, MetricsSink: metrics},
		staticProvider{snapshots: []ws.TraderSnapshot{{TraderID: "t-1", SessionID: "s-1", State: "active", LastHeartbeatUnix: time.Now().UTC().Unix()}}},
		&countingAssignment{},
		slog.Default(),
	)

	now := time.Now().UTC()
	engine.RunCycleForTest(now)

	if metrics.cycleCount.Load() != 1 {
		t.Fatalf("expected metrics cycle count 1, got %d", metrics.cycleCount.Load())
	}
	if metrics.candidateCount.Load() != 1 {
		t.Fatalf("expected metrics candidate count 1, got %d", metrics.candidateCount.Load())
	}
	if metrics.lastRunUnix.Load() != now.Unix() {
		t.Fatalf("expected metrics last run unix %d, got %d", now.Unix(), metrics.lastRunUnix.Load())
	}
}

func TestSelectCandidatesByTaskType_TradeEligibility(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	candidates := SelectCandidatesByTaskType([]ws.TraderSnapshot{
		{TraderID: "t-monitor", SessionID: "s-monitor", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "monitor", LoadIndex: 0.1, EffectiveExchanges: []string{"binance"}},
		{TraderID: "t-trade", SessionID: "s-trade", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.3, EffectiveExchanges: []string{"binance"}},
		{TraderID: "t-both", SessionID: "s-both", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "both", LoadIndex: 0.2, EffectiveExchanges: []string{"binance"}},
	}, now, 5*time.Second, taskTypeTrade)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates for trade task, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-both" {
		t.Fatalf("expected lowest load first for trade task (t-both), got %s", candidates[0].TraderID)
	}
	if candidates[1].TraderID != "t-trade" {
		t.Fatalf("expected second candidate t-trade, got %s", candidates[1].TraderID)
	}
	if candidates[0].Score >= candidates[1].Score {
		t.Fatalf("expected score of first candidate to be lower: first=%f second=%f", candidates[0].Score, candidates[1].Score)
	}
}

func TestSelectCandidatesByTaskType_MonitorPriority(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	candidates := SelectCandidatesByTaskType([]ws.TraderSnapshot{
		{TraderID: "t-trade-low", SessionID: "s1", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", TradeLoadIndex: 0.01},
		{TraderID: "t-both-mid", SessionID: "s2", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "both", TradeLoadIndex: 0.02},
		{TraderID: "t-monitor-high", SessionID: "s3", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "monitor", TradeLoadIndex: 0.30},
	}, now, 5*time.Second, taskTypeMonitor)

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates for monitor task, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-monitor-high" {
		t.Fatalf("expected monitor role priority first, got %s", candidates[0].TraderID)
	}
	if candidates[1].TraderID != "t-both-mid" {
		t.Fatalf("expected both role second, got %s", candidates[1].TraderID)
	}
	if candidates[2].TraderID != "t-trade-low" {
		t.Fatalf("expected trade role third, got %s", candidates[2].TraderID)
	}
}

func TestSelectCandidatesByTaskType_DeterministicTieBreak(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	candidates := SelectCandidatesByTaskType([]ws.TraderSnapshot{
		{TraderID: "trader-b", SessionID: "s-b", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "both", LoadIndex: 0.2, EffectiveExchanges: []string{"binance"}},
		{TraderID: "trader-a", SessionID: "s-a", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "both", LoadIndex: 0.2, EffectiveExchanges: []string{"binance"}},
	}, now, 5*time.Second, taskTypeTrade)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].TraderID != "trader-a" || candidates[1].TraderID != "trader-b" {
		t.Fatalf("expected deterministic trader_id ordering, got %s then %s", candidates[0].TraderID, candidates[1].TraderID)
	}
}

func TestSelectCandidatesByTaskType_TradeLatencyPenaltyWhenNoEffectiveExchanges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	candidates := SelectCandidatesByTaskType([]ws.TraderSnapshot{
		{TraderID: "t-with-latency", SessionID: "s1", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.4, EffectiveExchanges: []string{"binance"}, LatencyProfileMs: 50},
		{TraderID: "t-missing-latency", SessionID: "s2", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.1, EffectiveExchanges: nil},
	}, now, 5*time.Second, taskTypeTrade)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-with-latency" {
		t.Fatalf("expected candidate with effective exchanges to win, got %s", candidates[0].TraderID)
	}
	if candidates[1].Score < missingLatencyProfilePenalty {
		t.Fatalf("expected missing latency profile penalty to affect score, got %f", candidates[1].Score)
	}
}

func TestScoreFormulas(t *testing.T) {
	tradeScore := scoreTrade(25, 0.5)
	if tradeScore != 275 {
		t.Fatalf("expected trade score 275, got %f", tradeScore)
	}

	monitorScore := scoreMonitor(0.4, 0.5, 100)
	// 1000*(0.4^2)=160, 200*(0.5^2)=50, +100 => 310
	if monitorScore != 310 {
		t.Fatalf("expected monitor score 310, got %f", monitorScore)
	}
}

func TestSelectCandidatesByTaskType_TradeUsesLatencyProfileMsFromTelemetry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	candidates := SelectCandidatesByTaskType([]ws.TraderSnapshot{
		{TraderID: "t-low-lat", SessionID: "s1", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.5, EffectiveExchanges: []string{"binance"}, LatencyProfileMs: 30},
		{TraderID: "t-high-lat", SessionID: "s2", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.5, EffectiveExchanges: []string{"binance"}, LatencyProfileMs: 200},
	}, now, 5*time.Second, taskTypeTrade)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-low-lat" {
		t.Fatalf("expected lower latency profile candidate first, got %s", candidates[0].TraderID)
	}
	if candidates[0].Score >= candidates[1].Score {
		t.Fatalf("expected first score lower than second, got %f >= %f", candidates[0].Score, candidates[1].Score)
	}
}

func TestSelectCandidatesForTask_TradeRequiredExchangesN(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	snapshots := []ws.TraderSnapshot{
		{
			TraderID:          "t-a",
			SessionID:         "s-a",
			State:             "active",
			LastHeartbeatUnix: now.Unix() - 1,
			Role:              "trade",
			LoadIndex:         0.30,
			ExchangeLatencies: map[string]float64{"binance": 40, "kucoin": 60, "bybit": 50},
		},
		{
			TraderID:          "t-b",
			SessionID:         "s-b",
			State:             "active",
			LastHeartbeatUnix: now.Unix() - 1,
			Role:              "trade",
			LoadIndex:         0.30,
			ExchangeLatencies: map[string]float64{"binance": 30, "kucoin": 45}, // bybit missing
		},
	}

	required := []string{"binance", "kucoin", "bybit"}
	candidates := SelectCandidatesForTask(snapshots, now, 5*time.Second, taskTypeTrade, required)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-a" {
		t.Fatalf("expected trader with full n-exchange coverage to win, got %s", candidates[0].TraderID)
	}
	if candidates[0].Score >= candidates[1].Score {
		t.Fatalf("expected first score lower than second, got %f >= %f", candidates[0].Score, candidates[1].Score)
	}
}

func TestRunCycle_UsesRequiredExchangesProviderForTrade(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	engine := NewEngine(
		Config{
			Interval:      time.Second,
			HealthyWindow: 5 * time.Second,
			RequiredExchangesProvider: staticRequirementsProvider{trade: []ExchangeRef{
				{ExchangeID: 1, ExchangeName: "binance"},
				{ExchangeID: 2, ExchangeName: "kucoin"},
				{ExchangeID: 3, ExchangeName: "bybit"},
			}},
		},
		staticProvider{snapshots: []ws.TraderSnapshot{
			{TraderID: "t-a", SessionID: "s-a", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.30, ExchangeLatencies: map[string]float64{"binance": 40, "kucoin": 60, "bybit": 50}},
			{TraderID: "t-b", SessionID: "s-b", State: "active", LastHeartbeatUnix: now.Unix() - 1, Role: "trade", LoadIndex: 0.30, ExchangeLatencies: map[string]float64{"binance": 30, "kucoin": 45}},
		}},
		&countingAssignment{},
		slog.Default(),
	)

	candidates := SelectCandidatesForTask(engine.provider.GetTraderSnapshots(), now, 5*time.Second, taskTypeTrade, engine.resolveRequiredExchanges(context.Background()))
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].TraderID != "t-a" {
		t.Fatalf("expected full coverage trader to win when provider supplies required exchanges, got %s", candidates[0].TraderID)
	}
}

func TestRunCycle_UsesMonitorSetForMonitorTask(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	engine := NewEngine(
		Config{
			Interval:      time.Second,
			HealthyWindow: 5 * time.Second,
			TaskType:      taskTypeMonitor,
			RequiredExchangesProvider: staticRequirementsProvider{monitor: []ExchangeRef{
				{ExchangeID: 2, ExchangeName: "kucoin"},
			}},
		},
		staticProvider{},
		&countingAssignment{},
		slog.Default(),
	)

	items := engine.resolveRequiredExchanges(context.Background())
	if len(items) != 1 || items[0] != "kucoin" {
		t.Fatalf("expected monitor required exchanges from provider, got %v", items)
	}

	// Ensure monitor flow still runs without trade-latency dependence.
	engine.RunCycleForTest(now)
}

func TestSelectLatencyTestTargets_OnlyHealthyWithExchanges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	targets := SelectLatencyTestTargets([]ws.TraderSnapshot{
		{TraderID: "t-1", SessionID: "s-1", State: "active", LastHeartbeatUnix: now.Unix() - 1, EffectiveExchanges: []string{"binance", "kucoin"}},
		{TraderID: "t-2", SessionID: "s-2", State: "active", LastHeartbeatUnix: now.Unix() - 20, EffectiveExchanges: []string{"binance"}},
		{TraderID: "t-3", SessionID: "s-3", State: "registered", LastHeartbeatUnix: now.Unix() - 1, EffectiveExchanges: []string{"binance"}},
		{TraderID: "t-4", SessionID: "s-4", State: "active", LastHeartbeatUnix: now.Unix() - 1, EffectiveExchanges: nil},
	}, now, 5*time.Second)

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].TraderID != "t-1" {
		t.Fatalf("expected t-1 target, got %s", targets[0].TraderID)
	}
}

func TestRunLatencySweep_DispatchesToHealthyTargets(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	dispatcher := &recordingLatencyDispatcher{}
	engine := NewEngine(
		Config{
			Interval:             time.Second,
			HealthyWindow:        5 * time.Second,
			LatencyCheckInterval: 20 * time.Minute,
			LatencyDispatcher:    dispatcher,
		},
		staticProvider{snapshots: []ws.TraderSnapshot{
			{TraderID: "t-1", SessionID: "s-1", State: "active", LastHeartbeatUnix: now.Unix() - 1, EffectiveExchanges: []string{"binance", "kucoin"}},
			{TraderID: "t-2", SessionID: "s-2", State: "active", LastHeartbeatUnix: now.Unix() - 100, EffectiveExchanges: []string{"binance"}},
		}},
		&countingAssignment{},
		slog.Default(),
	)

	engine.RunLatencySweepForTest(now)
	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 dispatch call, got %d", len(calls))
	}
	if calls[0].sessionID != "s-1" || calls[0].traderID != "t-1" {
		t.Fatalf("unexpected dispatch target: %+v", calls[0])
	}
	if len(calls[0].exchanges) != 2 {
		t.Fatalf("expected 2 exchanges in dispatch, got %v", calls[0].exchanges)
	}
}
