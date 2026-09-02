package plugins

import (
	"context"

	"github.com/davidadel66/evie/internal/tools"
)

const (
	TodoPluginID         PluginID     = "todo"
	TodoListCapabilityID CapabilityID = "todo.list"
	TodoAddCapabilityID  CapabilityID = "todo.add"

	todoImplementationVersion = "1.0.0"
	todoContractVersion       = "1.0.0"
)

type Todo struct{}

func NewTodo() Todo { return Todo{} }

func (Todo) Start(context.Context) error { return nil }

func (Todo) Stop(context.Context) error { return nil }

func (Todo) Manifest() Manifest {
	return Manifest{
		ID:                    TodoPluginID,
		ImplementationVersion: todoImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: todoContractVersion},
			{ID: TodoAddCapabilityID, Version: todoContractVersion},
		},
	}
}

func (Todo) ToolCapabilities() []ToolCapability {
	todoTools := tools.TodoTools()
	return []ToolCapability{
		{ID: TodoListCapabilityID, ContractVersion: todoContractVersion, Tool: todoTools[0]},
		{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, Tool: todoTools[1]},
	}
}
