package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (m *Manager) writeAtomically(data []byte) error {
	tmpPath := m.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, m.stateFile); err != nil {
		return fmt.Errorf("rename temp state file: %w", err)
	}
	return nil
}

func (m *Manager) Backup() error {
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read state for backup: %w", err)
	}

	stamp := time.Now().UTC().Format("20060102_150405")
	backupPath := filepath.Join(m.backupDir, "daemon.state."+stamp)
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("write state backup: %w", err)
	}

	if err := m.cleanupOldBackups(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) cleanupOldBackups() error {
	files, err := filepath.Glob(filepath.Join(m.backupDir, "daemon.state.*"))
	if err != nil {
		return fmt.Errorf("list backup files: %w", err)
	}
	sort.Strings(files)
	if len(files) <= m.maxBackups {
		return nil
	}
	for _, oldFile := range files[:len(files)-m.maxBackups] {
		if err := os.Remove(oldFile); err != nil {
			return fmt.Errorf("remove old backup %s: %w", oldFile, err)
		}
	}
	return nil
}
