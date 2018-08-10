package main

import (
	"fmt"

	"github.com/Xfennec/mulch"
)

// Log provides error/warning/etc helpers for a Hub
type Log struct {
	target string
	hub    *Hub
}

// NewLog creates a new log for the provided target and hub
// note: mulch.MessageNoTarget is an acceptable target
func NewLog(target string, hub *Hub) *Log {
	return &Log{
		target: target,
		hub:    hub,
	}
}

// Log is a low-level function for sending a Message
func (log *Log) Log(message *mulch.Message) {
	// TODO: use our own *log.Logger (see log.go in Nosee project)
	fmt.Printf("%s(%s): %s\n", message.Type, message.Target, message.Message)
	message.Target = log.target
	log.hub.Broadcast(message)
}

// Error sends a MessageError Message
func (log *Log) Error(message string) {
	log.Log(mulch.NewMessage(mulch.MessageError, log.target, message))
}

// Warning sends a MessageWarning Message
func (log *Log) Warning(message string) {
	log.Log(mulch.NewMessage(mulch.MessageWarning, log.target, message))
}

// Info sends an MessageInfo Message
func (log *Log) Info(message string) {
	log.Log(mulch.NewMessage(mulch.MessageInfo, log.target, message))
}

// Trace sends an MessageTrace Message
func (log *Log) Trace(message string) {
	log.Log(mulch.NewMessage(mulch.MessageTrace, log.target, message))
}

// SetTarget change the current "sending" target
func (log *Log) SetTarget(target string) {
	log.target = target
}
