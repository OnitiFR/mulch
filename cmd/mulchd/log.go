package main

import (
	"fmt"

	"github.com/Xfennec/mulch"
)

type Log struct {
	target string
	hub    *Hub
}

func NewLog(target string, hub *Hub) *Log {
	return &Log{
		target: target,
		hub:    hub,
	}
}

func (log *Log) Log(message *mulch.Message) {
	// TODO: use our own *log.Logger (see log.go in Nosee project)
	fmt.Printf("%s(%s): %s\n", message.Type, message.Target, message.Message)
	message.Target = log.target
	log.hub.Broadcast(message)
}

func (log *Log) Error(message string) {
	log.Log(mulch.NewMessage(mulch.MessageError, log.target, message))
}

func (log *Log) Warning(message string) {
	log.Log(mulch.NewMessage(mulch.MessageWarning, log.target, message))
}

func (log *Log) Info(message string) {
	log.Log(mulch.NewMessage(mulch.MessageInfo, log.target, message))
}

func (log *Log) Trace(message string) {
	log.Log(mulch.NewMessage(mulch.MessageTrace, log.target, message))
}

func (log *Log) SetTarget(target string) {
	log.target = target
}
