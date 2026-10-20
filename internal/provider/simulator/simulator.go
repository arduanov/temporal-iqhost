package simulator

import (
	"context"
	"sync"
	"time"

	"temporal-iqhost/internal/provider"
)

// Simulator хранит состояние в памяти; NewPersistent также сохраняет его в файл.
type Simulator struct {
	mutex       sync.Mutex
	states      map[string]provider.State
	generations map[string]int64
	results     map[string]commandResult
	delays      Delays
	stateFile   string
}

type commandResult struct {
	request provider.Request
	state   provider.State
}

var _ provider.Provider = (*Simulator)(nil)

func New() *Simulator {
	return NewWithDelays(Delays{
		Create:        3 * time.Second,
		AttachNetwork: time.Second,
		Start:         2 * time.Second,
		WaitReady:     3 * time.Second,
		Stop:          2 * time.Second,
		DetachNetwork: time.Second,
		Delete:        2 * time.Second,
	})
}

func NewWithDelays(delays Delays) *Simulator {
	return &Simulator{
		states:      make(map[string]provider.State),
		generations: make(map[string]int64),
		results:     make(map[string]commandResult),
		delays:      delays,
	}
}

func (s *Simulator) run(ctx context.Context, request provider.Request, step string) (provider.State, error) {
	s.mutex.Lock()
	state, found, err := s.check(ctx, request, step)
	s.mutex.Unlock()
	if err != nil || found {
		return state, err
	}

	// Ожидание не держит блокировку: Fence и команды других VM проходят сразу.
	if err = wait(ctx, s.delays.forStep(step)); err != nil {
		return provider.State{}, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	// За время ожидания могли смениться владелец, состояние или завершиться повтор.
	state, found, err = s.check(ctx, request, step)
	if err != nil || found {
		return state, err
	}
	if err := s.save(request, state, step); err != nil {
		return provider.State{}, err
	}
	if step == provider.StepFence {
		s.generations[request.VMID] = request.Generation
	} else if step != provider.StepInspect {
		s.states[request.VMID] = state
	}
	// Эффект и результат фиксируются вместе. Потерянный ответ восстановит повтор.
	s.results[request.Key] = commandResult{request: request, state: state}
	return state, nil
}

// check вызывается под mutex; вычисляет новое состояние, не меняя карты.
func (s *Simulator) check(ctx context.Context, request provider.Request, step string) (provider.State, bool, error) {
	if err := ctx.Err(); err != nil {
		return provider.State{}, false, err
	}
	if request.Step != step || request.Key != request.OperationID+"/"+request.Phase+"/"+step || request.Generation < 1 {
		return provider.State{}, false, provider.Permanent("invalid command identity")
	}
	generation := s.generations[request.VMID]
	if request.Generation < generation || (step != provider.StepFence && request.Generation != generation) {
		return provider.State{}, false, provider.Permanent("stale or unfenced generation")
	}
	// Даже сохранённый ответ старого поколения нельзя выдавать после Fence.
	if previous, found := s.results[request.Key]; found {
		if previous.request != request {
			return provider.State{}, false, provider.Permanent("command content conflict")
		}
		return previous.state, true, nil
	}
	state := s.states[request.VMID]
	err := applyCommand(request, step, &state)
	return state, false, err
}
