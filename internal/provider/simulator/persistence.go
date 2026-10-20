package simulator

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"temporal-iqhost/internal/provider"
)

type snapshotResult struct {
	Request provider.Request
	State   provider.State
}

type snapshot struct {
	States      map[string]provider.State
	Generations map[string]int64
	Results     map[string]snapshotResult
}

// NewPersistent восстанавливает независимое состояние провайдера из файла.
// Файл принадлежит одному процессу симулятора; совместное использование запрещено.
func NewPersistent(path string) (*Simulator, error) {
	adapter := New()
	adapter.stateFile = path
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return adapter, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read simulator snapshot: %w", err)
	}
	var saved snapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("decode simulator snapshot: %w", err)
	}
	if saved.States == nil || saved.Generations == nil || saved.Results == nil {
		return nil, fmt.Errorf("incomplete simulator snapshot")
	}
	adapter.states = saved.States
	adapter.generations = saved.Generations
	for key, result := range saved.Results {
		adapter.results[key] = commandResult{request: result.Request, state: result.State}
	}
	return adapter, nil
}

// Сначала сохраняем будущее состояние, затем run меняет память и отвечает клиенту.
// При ошибке записи команда не применяется и может быть безопасно повторена.
// Временный файл и rename не дают перезапуску прочитать половину JSON.
func (s *Simulator) save(request provider.Request, state provider.State, step string) error {
	if s.stateFile == "" {
		return nil
	}
	saved := snapshot{
		States:      maps.Clone(s.states),
		Generations: maps.Clone(s.generations),
		Results:     make(map[string]snapshotResult, len(s.results)+1),
	}
	for key, result := range s.results {
		saved.Results[key] = snapshotResult{Request: result.request, State: result.state}
	}
	if step == provider.StepFence {
		saved.Generations[request.VMID] = request.Generation
	} else if step != provider.StepInspect {
		saved.States[request.VMID] = state
	}
	saved.Results[request.Key] = snapshotResult{Request: request, State: state}
	file, err := os.CreateTemp(filepath.Dir(s.stateFile), ".snapshot-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err := json.NewEncoder(file).Encode(saved); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.stateFile)
}
