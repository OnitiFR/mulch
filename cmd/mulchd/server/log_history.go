package server

import (
	"fmt"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// LogHistory stores messages in a fixed size rotating buffer
type LogHistory struct {
	size      int
	rotations int
	nextEntry int
	messages  []*common.Message
	mux       sync.Mutex
}

// NewLogHistory will create and initialize a new log message history
func NewLogHistory(elems int) *LogHistory {
	return &LogHistory{
		size:      elems,
		rotations: 0,
		nextEntry: 0,
		messages:  make([]*common.Message, elems),
	}
}

// Push a new message in the buffer
func (ml *LogHistory) Push(message *common.Message) {
	ml.mux.Lock()
	defer ml.mux.Unlock()

	ml.messages[ml.nextEntry] = message
	ml.nextEntry++
	if ml.nextEntry >= ml.size {
		ml.nextEntry = 0
		ml.rotations++
	}
}

// Dump all logs in the buffer (temporary test)
func (ml *LogHistory) Dump() {
	ml.mux.Lock()
	defer ml.mux.Unlock()

	if ml.rotations == 0 {
		for index := 0; index < ml.nextEntry; index++ {
			fmt.Println(ml.messages[index])
		}
		return
	}

	// … at least one rotation:

	if ml.nextEntry > 0 {
		for index := ml.nextEntry; index < ml.size; index++ {
			fmt.Println(ml.messages[index])
		}
	}

	end := ml.nextEntry - 1
	if end == -1 {
		end = ml.size - 1
	}

	for index := 0; index <= end; index++ {
		fmt.Println(ml.messages[index])
	}

}
