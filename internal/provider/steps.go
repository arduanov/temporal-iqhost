package provider

// Infrastructure command steps shared by orchestration and provider implementations.
// Values are part of persisted command identities and must remain stable.
const (
	StepFence          = "fence"
	StepCreate         = "create"
	StepAttachNetwork  = "attach-network"
	StepStart          = "start"
	StepWaitReady      = "wait-ready"
	StepStop           = "stop"
	StepDetachNetwork  = "detach-network"
	StepDeleteInstance = "delete-instance"
	StepInspect        = "inspect"
)
