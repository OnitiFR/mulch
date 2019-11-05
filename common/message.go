package common

import (
	"time"
)

// Messages are the glue between the client ('mulch' command) and the
// server ('mulchd'), so this package is shared using a common package
// named 'mulch'.

// TODO: define types for Targets and Types

// SUCCESS & FAILURE will end a client connection (no?)
const (
	MessageSuccess = "SUCCESS"
	MessageFailure = "FAILURE"
)

// Message types
const (
	MessageError   = "ERROR"
	MessageWarning = "WARNING"
	MessageInfo    = "INFO"
	MessageTrace   = "TRACE"
	MessageNoop    = "NOOP" // MessageNoop is used for keep-alive messages
)

// Special message targets
const (
	MessageNoTarget   = ""
	MessageAllTargets = "*"
)

// Message describe a message between mulch client and mulchd server
type Message struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Target  string    `json:"target"`
	Message string    `json:"message"`
}

// NewMessage creates a new Message instance
func NewMessage(mtype string, target string, message string) *Message {
	return &Message{
		Time:    time.Now(),
		Type:    mtype,
		Target:  target,
		Message: message,
	}
}

// MessageMatchTarget returns true if the message matches the target
func MessageMatchTarget(message *Message, target string) bool {
	if target != message.Target &&
		message.Target != MessageNoTarget &&
		target != MessageAllTargets {
		return false // not for this client
	}
	return true
}
