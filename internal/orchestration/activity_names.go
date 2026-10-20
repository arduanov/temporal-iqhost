package orchestration

import (
	"context"

	"go.temporal.io/sdk/activity"

	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

// Names describe the individual operations in Temporal history and UI.
var stepActivityNames = map[string]string{
	vm.StepBegin:                "vm.begin-operation",
	vm.StepReserve:              "vm.reserve-resources",
	provider.StepCreate:         "vm.create-instance",
	provider.StepAttachNetwork:  "vm.attach-network",
	provider.StepStart:          "vm.start-instance",
	provider.StepWaitReady:      "vm.wait-ready",
	provider.StepStop:           "vm.stop-instance",
	provider.StepDetachNetwork:  "vm.detach-network",
	provider.StepDeleteInstance: "vm.delete-instance",
	vm.StepRelease:              "vm.release-resources",
	vm.StepBeginRollback:        "vm.begin-rollback",
	vm.StepFinish:               "vm.finish-operation",
}

func stepActivityName(step string) string {
	name, ok := stepActivityNames[step]
	if !ok {
		panic("unknown workflow step: " + step)
	}
	return name
}

// RegisterStepActivities exposes the shared step handler under each activity name.
func RegisterStepActivities(registry interface {
	RegisterActivityWithOptions(interface{}, activity.RegisterOptions)
}, handler func(context.Context, vm.StepRequest) (vm.StepResult, error),
) {
	for _, name := range stepActivityNames {
		registry.RegisterActivityWithOptions(handler, activity.RegisterOptions{Name: name})
	}
}
