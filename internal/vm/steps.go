package vm

// Local operation steps. Values are persisted and must remain stable.
const (
	StepBegin         = "begin"
	StepBeginRollback = "begin-rollback"
	StepReserve       = "reserve"
	StepRelease       = "release"
	StepFinish        = "finish"
)
