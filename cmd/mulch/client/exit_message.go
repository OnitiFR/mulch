package client

import "fmt"

// ExitMessage is displayed during client exit
type ExitMessage struct {
	Disabled bool
	Message  string
}

var globalExitMessage ExitMessage

// InitExitMessage resets global ExitMessage
func InitExitMessage() {
	globalExitMessage = ExitMessage{
		Disabled: false,
		Message:  "",
	}
}

// GetExitMessage return a pointer to the global ExitMessage
func GetExitMessage() *ExitMessage {
	return &globalExitMessage
}

// Disable global ExitMessage
func (em *ExitMessage) Disable() {
	em.Disabled = true
}

// Display global ExitMessage
func (em *ExitMessage) Display() {
	if em.Disabled || em.Message == "" {
		return
	}
	fmt.Println()
	fmt.Print(em.Message)
}
