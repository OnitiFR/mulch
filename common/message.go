package common

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatih/color"
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

// Message.Print options
const (
	MessagePrintTime     = true
	MessagePrintNoTime   = false
	MessagePrintTarget   = true
	MessagePrintNoTarget = false
)

// Message.MatchTarget options
const (
	MessageMatchDefault = false
	MessageMatchExact   = true
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

// MatchTarget returns true if the message matches the target
// exact searches are usefull for specific target logs (excluding all "global" messages)
func (message *Message) MatchTarget(target string, exact bool) bool {
	if exact == true && target != message.Target {
		return false
	}

	if target != message.Target &&
		message.Target != MessageNoTarget &&
		target != MessageAllTargets {
		return false // not for this client
	}

	return true
}

// Print the formatted message
func (message *Message) Print(showTime bool, showTarget bool) error {
	var retError error

	// the longest types are 7 chars wide
	mtype := fmt.Sprintf("% -7s", message.Type)
	content := message.Message

	switch message.Type {
	case MessageTrace:
		c := color.New(color.FgWhite).SprintFunc()
		content = c(content)
		mtype = c(mtype)
	case MessageInfo:
	case MessageWarning:
		c := color.New(color.FgYellow).SprintFunc()
		content = c(content)
		mtype = c(mtype)
	case MessageError:
		c := color.New(color.FgRed).SprintFunc()
		content = c(content)
		mtype = c(mtype)
	case MessageFailure:
		retError = errors.New("Exiting with failure status due to previous errors")
		c := color.New(color.FgHiRed).SprintFunc()
		content = c(content)
		mtype = c(mtype)
	case MessageSuccess:
		c := color.New(color.FgHiGreen).SprintFunc()
		content = c(content)
		mtype = c(mtype)
	}

	time := ""
	if showTime {
		time = message.Time.Format("15:04:05") + " "
	}

	target := ""
	if showTarget {
		target = " [" + message.Target + "] "
	}

	fmt.Printf("%s%s%s: %s\n", time, target, mtype, content)

	return retError

}
