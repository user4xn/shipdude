package consts

// StackType identifies the application runtime detected in a repository.
type StackType string

const (
	StackGo      StackType = "go"
	StackNode    StackType = "node"
	StackPython  StackType = "python"
	StackUnknown StackType = "unknown"
)

// AuthMethod identifies how ShipDude authenticates against a target VPS.
type AuthMethod string

const (
	AuthPassword AuthMethod = "password"
	AuthKey      AuthMethod = "key"
)

// ContainerAction is a lifecycle command applied to a running project container.
type ContainerAction string

const (
	ActionStart   ContainerAction = "start"
	ActionStop    ContainerAction = "stop"
	ActionRestart ContainerAction = "restart"
)

// IsValid reports whether the action is a supported container command.
func (a ContainerAction) IsValid() bool {
	switch a {
	case ActionStart, ActionStop, ActionRestart:
		return true
	default:
		return false
	}
}

// DeployStatus reports the state of an asynchronous job kick-off.
type DeployStatus string

const (
	StatusStarted   DeployStatus = "started"
	StatusTriggered DeployStatus = "triggered"
	StatusSuccess   DeployStatus = "success"
	StatusDeleted   DeployStatus = "deleted"
)
