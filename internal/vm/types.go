package vm

type VMState string

const (
	NotCreated VMState = "not-created"
	Stopped    VMState = "stopped"
	VMRunning  VMState = "running"
	Deleted    VMState = "deleted"
	Unknown    VMState = "unknown"
)

type OperationKind string

const (
	CreateVM OperationKind = "create"
	StartVM  OperationKind = "start"
	StopVM   OperationKind = "stop"
	DeleteVM OperationKind = "delete"
	Cleanup  OperationKind = "cleanup"
)

type OperationStatus string

const (
	Pending           OperationStatus = "pending"
	InProgress        OperationStatus = "running"
	Succeeded         OperationStatus = "succeeded"
	OperationFailed   OperationStatus = "failed"
	OperationCanceled OperationStatus = "canceled"
	Blocked           OperationStatus = "rollback-blocked"
)

type Phase string

const (
	Forward       Phase = "forward"
	RollbackPhase Phase = "rollback"
)

type Command struct {
	UserID    string        `json:"user_id"`
	VMID      string        `json:"vm_id"`
	Kind      OperationKind `json:"kind"`
	CPU       int32         `json:"cpu,omitempty"`
	MemoryMiB int32         `json:"memory_mib,omitempty"`
	DiskGiB   int32         `json:"disk_gib,omitempty"`
	GPU       bool          `json:"gpu,omitempty"`
}
type Operation struct {
	UserID            string          `json:"user_id"`
	ID                string          `json:"id"`
	VMID              string          `json:"vm_id"`
	Kind              OperationKind   `json:"kind"`
	Status            OperationStatus `json:"status"`
	Phase             Phase           `json:"phase"`
	Step              string          `json:"step"`
	WorkflowID        string          `json:"workflow_id"`
	RunID             string          `json:"run_id"`
	Error             string          `json:"error"`
	RecoveryTarget    VMState         `json:"recovery_target"`
	SourceOperationID string          `json:"source_operation_id,omitempty"`
}

func (o Operation) Terminal() bool { return o.Status != Pending && o.Status != InProgress }

type StepRequest struct {
	OperationID string
	Step        string
	Status      OperationStatus
	Error       string
}
type StepResult struct {
	Kind   OperationKind
	Target VMState
	Noop   bool
}

// Delivery is independent of database driver types.
type Delivery struct {
	ID        int64
	Operation Operation
	Token     string
	Attempts  int32
}
