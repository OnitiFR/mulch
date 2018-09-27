package main

// This log system is a stripped version of Mulch message system.
// (no target, no hub)

import (
	"fmt"
	"time"
)

// Message types
const (
	MessageError   = "ERROR"
	MessageWarning = "WARNING"
	MessageInfo    = "INFO"
	MessageTrace   = "TRACE"
)

// Message is a Log message
type Message struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Message string    `json:"message"`
}

// NewMessage instanciates a new Message
func NewMessage(mtype string, message string) *Message {
	return &Message{
		Time:    time.Now(),
		Type:    mtype,
		Message: message,
	}
}

// Log provides error/warning/etc helpers for a Hub
type Log struct {
	trace bool
}

// NewLog creates a new log
func NewLog(trace bool) *Log {
	return &Log{trace: trace}
}

// Log is a low-level function for sending a Message
func (log *Log) Log(message *Message) {
	if message.Type == MessageTrace && log.trace == false {
		return
	}
	fmt.Printf("%s: %s\n", message.Type, message.Message)
}

// Error sends a MessageError Message
func (log *Log) Error(message string) {
	log.Log(NewMessage(MessageError, message))
}

// Errorf sends a formated string MessageError Message
func (log *Log) Errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Error(msg)
}

// Warning sends a MessageWarning Message
func (log *Log) Warning(message string) {
	log.Log(NewMessage(MessageWarning, message))
}

// Warningf sends a formated string MessageWarning Message
func (log *Log) Warningf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Warning(msg)
}

// Info sends an MessageInfo Message
func (log *Log) Info(message string) {
	log.Log(NewMessage(MessageInfo, message))
}

// Infof sends a formated string MessageInfo Message
func (log *Log) Infof(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Info(msg)
}

// Trace sends an MessageTrace Message
func (log *Log) Trace(message string) {
	log.Log(NewMessage(MessageTrace, message))
}

// Tracef sends a formated string MessageTrace Message
func (log *Log) Tracef(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Trace(msg)
}
