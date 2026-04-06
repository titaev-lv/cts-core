package rest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

type stubTraderSessionRepo struct {
	createErrs  []error
	createCalls int

	activeByTrader map[int]*models.TraderSession
	activeErr      error

	endErr   error
	endCalls []endCall
}

type endCall struct {
	sessionID string
	reason    models.DisconnectReason
	errorMsg  *string
}

func (s *stubTraderSessionRepo) Create(_ context.Context, _ *models.TraderSession) error {
	s.createCalls++
	idx := s.createCalls - 1
	if idx >= 0 && idx < len(s.createErrs) {
		return s.createErrs[idx]
	}
	return nil
}

func (s *stubTraderSessionRepo) GetByID(_ context.Context, _ int64) (*models.TraderSession, error) {
	return nil, nil
}

func (s *stubTraderSessionRepo) GetBySessionID(_ context.Context, _ string) (*models.TraderSession, error) {
	return nil, nil
}

func (s *stubTraderSessionRepo) GetActiveByTraderID(_ context.Context, traderID int) (*models.TraderSession, error) {
	if s.activeErr != nil {
		return nil, s.activeErr
	}
	if s.activeByTrader == nil {
		return nil, nil
	}
	return s.activeByTrader[traderID], nil
}

func (s *stubTraderSessionRepo) ListActive(_ context.Context) ([]*models.TraderSession, error) {
	return nil, nil
}

func (s *stubTraderSessionRepo) UpdateHeartbeat(_ context.Context, _ string) error {
	return nil
}

func (s *stubTraderSessionRepo) EndSession(_ context.Context, sessionID string, reason models.DisconnectReason, errorMsg *string) error {
	if s.endErr != nil {
		return s.endErr
	}
	s.endCalls = append(s.endCalls, endCall{sessionID: sessionID, reason: reason, errorMsg: errorMsg})
	return nil
}

func (s *stubTraderSessionRepo) CleanupOldSessions(_ context.Context, _ int) (int64, error) {
	return 0, nil
}

func duplicateEntryErr() error {
	return fmt.Errorf("insert failed: %w", &mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry"})
}

func TestCreateSession_RecoversStaleActiveSession(t *testing.T) {
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	repo := &stubTraderSessionRepo{
		createErrs: []error{duplicateEntryErr(), nil},
		activeByTrader: map[int]*models.TraderSession{
			7: {
				TraderID:       7,
				SessionID:      "old-stale-session",
				ConnectedAt:    now.Add(-10 * time.Minute),
				LastHeartbeat:  now.Add(-6 * time.Minute),
				EndedAt:        sql.NullTime{},
				ErrorMessage:   sql.NullString{},
				DisconnectReason: sql.NullString{},
			},
		},
	}

	p := &wsSessionPersistence{traderSessionRepo: repo, staleAfter: 3 * time.Minute}
	err := p.CreateSession(context.Background(), ws.SessionCreateInput{
		TraderID:       7,
		SessionID:      "new-session",
		WSConnectionID: "ws-1",
		IPAddress:      "127.0.0.1",
		ConnectedAt:    now,
		LastHeartbeat:  now,
	})
	if err != nil {
		t.Fatalf("expected stale session recovery success, got error: %v", err)
	}
	if repo.createCalls != 2 {
		t.Fatalf("expected create called twice (initial+retry), got %d", repo.createCalls)
	}
	if len(repo.endCalls) != 1 {
		t.Fatalf("expected one stale session finalization, got %d", len(repo.endCalls))
	}
	if repo.endCalls[0].sessionID != "old-stale-session" {
		t.Fatalf("expected ended stale session old-stale-session, got %q", repo.endCalls[0].sessionID)
	}
	if repo.endCalls[0].reason != models.DisconnectTimeout {
		t.Fatalf("expected disconnect reason timeout, got %q", repo.endCalls[0].reason)
	}
}

func TestCreateSession_FreshActiveSessionReturnsDuplicate(t *testing.T) {
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	repo := &stubTraderSessionRepo{
		createErrs: []error{duplicateEntryErr()},
		activeByTrader: map[int]*models.TraderSession{
			9: {
				TraderID:      9,
				SessionID:     "fresh-session",
				ConnectedAt:   now.Add(-2 * time.Minute),
				LastHeartbeat: now.Add(-30 * time.Second),
			},
		},
	}

	p := &wsSessionPersistence{traderSessionRepo: repo, staleAfter: 3 * time.Minute}
	err := p.CreateSession(context.Background(), ws.SessionCreateInput{
		TraderID:      9,
		SessionID:     "new-session",
		ConnectedAt:   now,
		LastHeartbeat: now,
	})
	if err == nil {
		t.Fatalf("expected duplicate connection error")
	}
	if !errors.Is(err, ws.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists, got %v", err)
	}
	if repo.createCalls != 1 {
		t.Fatalf("expected single create attempt, got %d", repo.createCalls)
	}
	if len(repo.endCalls) != 0 {
		t.Fatalf("expected no stale session finalization, got %d", len(repo.endCalls))
	}
}

func TestIsSessionStale_Boundary(t *testing.T) {
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
	s := &models.TraderSession{ConnectedAt: now.Add(-10 * time.Minute), LastHeartbeat: now.Add(-3 * time.Minute)}

	if !isSessionStale(s, now, 3*time.Minute) {
		t.Fatalf("expected session to be stale at exact boundary")
	}
	if isSessionStale(s, now, 4*time.Minute) {
		t.Fatalf("expected session to be fresh with larger stale window")
	}
}
