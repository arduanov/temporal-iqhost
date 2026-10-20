// Package provider задаёт синхронный контракт команд инфраструктурного адаптера.
// Промышленный адаптер должен сохранять результаты и Fence между перезапусками.
// Демо-симулятор в Compose сохраняет состояние в отдельном volume;
// без --state-file его состояние ограничено временем жизни процесса.
package provider

import "context"

// Request is immutable for a command key (operation/phase/step). Adapters must
// persist both request and result; retries return the original result and a
// changed request is a permanent error. No database or Temporal types cross this boundary.
type Request struct {
	VMID        string
	OperationID string
	Phase       string
	Step        string
	Generation  int64
	Key         string
	Config      Config
	Resources   Resources
}
type Config struct {
	CPU, MemoryMiB, DiskGiB int32
	GPU                     bool
}
type Resources struct {
	NodeID, GPUID, DiskID, PoolID int64
	IP                            string
}
type State struct {
	Exists, NetworkAttached, PoweredOn bool
	NodeID                             int64
}

// Success confirms completion, not acceptance. Delete confirms absence. An
// asynchronous adapter must persist its remote task ID and await that same task
// on retry. Fence must prevent ALL older commands from causing later effects,
// including already accepted tasks, before returning success. Missing resources
// are successful no-ops for Stop, DetachNetwork and Delete.
type Provider interface {
	Fence(context.Context, Request) error
	Create(context.Context, Request) (State, error)
	AttachNetwork(context.Context, Request) (State, error)
	Start(context.Context, Request) (State, error)
	WaitReady(context.Context, Request) (State, error)
	Stop(context.Context, Request) (State, error)
	DetachNetwork(context.Context, Request) (State, error)
	Delete(context.Context, Request) (State, error)
	Inspect(context.Context, Request) (State, error)
}

// Other errors are transient; context cancellation must remain discoverable.
type PermanentError struct{ Message string }

func (e *PermanentError) Error() string { return e.Message }
func Permanent(message string) error    { return &PermanentError{message} }
