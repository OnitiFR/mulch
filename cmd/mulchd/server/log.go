package server

import (
	"fmt"

	"github.com/OnitiFR/mulch/common"
)

// Log provides error/warning/etc helpers for a Hub
type Log struct {
	target  string
	hub     *Hub
	history *LogHistory
}

// NewLog creates a new log for the provided target and hub
// note: common.MessageNoTarget is an acceptable target
func NewLog(target string, hub *Hub, history *LogHistory) *Log {
	return &Log{
		target:  target,
		hub:     hub,
		history: history,
	}
}

// Log is a low-level function for sending a Message
func (log *Log) Log(message *common.Message) {
	message.Target = log.target

	if !(message.Type == common.MessageTrace && !log.hub.trace) {
		// TODO: use our own *log.Logger (see log.go in Nosee project)
		fmt.Printf("%s(%s): %s\n", message.Type, message.Target, message.Message)
	}

	// we don't historize NOOP and TRACE messages
	if message.Type != common.MessageNoop && message.Type != common.MessageTrace {
		log.history.Push(message)
	}

	log.hub.Broadcast(message)
}

// Error sends a MessageError Message
func (log *Log) Error(message string) {
	log.Log(common.NewMessage(common.MessageError, log.target, message))
}

// Errorf sends a formated string MessageError Message
func (log *Log) Errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Error(msg)
}

// Warning sends a MessageWarning Message
func (log *Log) Warning(message string) {
	log.Log(common.NewMessage(common.MessageWarning, log.target, message))
}

// Warningf sends a formated string MessageWarning Message
func (log *Log) Warningf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Warning(msg)
}

// Info sends an MessageInfo Message
func (log *Log) Info(message string) {
	log.Log(common.NewMessage(common.MessageInfo, log.target, message))
}

// Infof sends a formated string MessageInfo Message
func (log *Log) Infof(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Info(msg)
}

// Trace sends an MessageTrace Message
func (log *Log) Trace(message string) {
	log.Log(common.NewMessage(common.MessageTrace, log.target, message))
}

// Tracef sends a formated string MessageTrace Message
func (log *Log) Tracef(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Trace(msg)
}

// Success sends an MessageSuccess Message
func (log *Log) Success(message string) {
	log.Log(common.NewMessage(common.MessageSuccess, log.target, message))
}

// Successf sends a formated string MessageSuccess Message
func (log *Log) Successf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Success(msg)
}

// Failure sends an MessageFailure Message
func (log *Log) Failure(message string) {
	log.Log(common.NewMessage(common.MessageFailure, log.target, message))
}

// Failuref sends a formated string MessageFailure Message
func (log *Log) Failuref(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Failure(msg)
}

// SetTarget change the current "sending" target
func (log *Log) SetTarget(target string) {
	// You can't send to "*", only listen. But NoTarget does the same
	// since since everybody receives it.
	if target == common.MessageAllTargets {
		target = common.MessageNoTarget
	}
	log.target = target
}
