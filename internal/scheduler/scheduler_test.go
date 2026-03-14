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
