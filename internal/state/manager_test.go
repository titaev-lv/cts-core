package state

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()
	mgr, err := NewManager(ManagerConfig{
		StateFile:    filepath.Join(tmpDir, "daemon.state"),
		BackupDir:    filepath.Join(tmpDir, "backups"),
		MaxBackups:   2,
		SyncInterval: 50 * time.Millisecond,
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return mgr
}

func TestLoadWhenFileMissing(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state := mgr.GetState()
	if state.Version == "" {
		t.Fatalf("expected state version to be set")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetServerStatus("running")
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	mgr2, err := NewManager(ManagerConfig{
		StateFile:  mgr.stateFile,
		BackupDir:  mgr.backupDir,
		MaxBackups: 2,
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() second instance error = %v", err)
	}
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load() second instance error = %v", err)
	}
	if got := mgr2.GetState().Server.Status; got != "running" {
		t.Fatalf("expected status=running, got %s", got)
	}
}

func TestLoadCorruptedFileFallback(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.WriteFile(mgr.stateFile, []byte("{broken-json"), 0o600); err != nil {
		t.Fatalf("write corrupted state: %v", err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := mgr.GetState().Version; got == "" {
		t.Fatalf("expected fallback state version, got empty")
	}
}

func TestBackupRotation(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	for i := 0; i < 4; i++ {
		time.Sleep(1 * time.Second)
		if err := mgr.Backup(); err != nil {
			t.Fatalf("Backup() error = %v", err)
		}
	}
	files, err := filepath.Glob(filepath.Join(mgr.backupDir, "daemon.state.*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) > mgr.maxBackups {
		t.Fatalf("expected backups <= %d, got %d", mgr.maxBackups, len(files))
	}
}

func TestUpdatedAtChangesOnMutations(t *testing.T) {
	mgr := newTestManager(t)
	before := mgr.GetState().UpdatedAt

	time.Sleep(10 * time.Millisecond)
	mgr.SetServerStatus("running")
	afterStatus := mgr.GetState().UpdatedAt
	if !afterStatus.After(before) {
		t.Fatalf("expected UpdatedAt to move forward after SetServerStatus")
	}

	time.Sleep(10 * time.Millisecond)
	mgr.SetRuntimeWS(3, time.Now().Unix())
	afterWS := mgr.GetState().UpdatedAt
	if !afterWS.After(afterStatus) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeWS")
	}
}
