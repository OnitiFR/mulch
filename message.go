package mulch

import "time"

// TODO: add server timestamp

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
)

// MessageNoop is used for keep-alive messages
const MessageNoop = "NOOP"

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
