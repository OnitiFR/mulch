package server

import (
	"fmt"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// maximum message length (message is truncated if too long)
const logHistoryMaxMessageLen = 256

type logHistorySlot struct {
	payload *common.Message
	older   *logHistorySlot
	newer   *logHistorySlot
}

// LogHistory stores messages in a limited size double chain list
type LogHistory struct {
	maxSize     int
	currentSize int
	oldest      *logHistorySlot
	newest      *logHistorySlot
	mux         sync.Mutex
}

// NewLogHistory will create and initialize a new log message history
func NewLogHistory(elems int) *LogHistory {
	return &LogHistory{
		maxSize: elems,
	}
}

// Push a new message in the list
func (lh *LogHistory) Push(message *common.Message) {
	lh.mux.Lock()
	defer lh.mux.Unlock()

	localMsg := message

	// truncate message if needed
	if len(message.Message) > logHistoryMaxMessageLen {
		dup := *message
		dup.Message = dup.Message[:logHistoryMaxMessageLen] + "…"
		localMsg = &dup
	}

	curr := &logHistorySlot{
		payload: localMsg,
	}

	if lh.currentSize == 0 {
		lh.newest = curr
		lh.oldest = curr
		lh.currentSize++
		return
	}

	// place "curr" in front
	lh.newest.newer = curr
	curr.older = lh.newest
	lh.newest = curr
	lh.currentSize++

	// remove the oldest slot
	if lh.currentSize > lh.maxSize {
		lh.oldest = lh.oldest.newer
		lh.oldest.older = nil
		lh.currentSize--
	}

}

// Dump all logs in the buffer (temporary test)
func (lh *LogHistory) Dump() {
	lh.mux.Lock()
	defer lh.mux.Unlock()

	fmt.Println(lh.currentSize)
	curr := lh.newest
	for curr != nil {
		fmt.Println(curr.payload)
		curr = curr.older
	}
}

// Search return an array of messages (latest messages, up to maxMessages, for a specific target)
func (lh *LogHistory) Search(maxMessages int, target string) []*common.Message {
	lh.mux.Lock()
	defer lh.mux.Unlock()

	reversedMessages := make([]*common.Message, maxMessages)
	exact := common.MessageMatchDefault
	if target != common.MessageAllTargets {
		exact = common.MessageMatchExact
	}

	curr := lh.newest
	count := 0
	for curr != nil && count < maxMessages {
		if !curr.payload.MatchTarget(target, exact) {
			curr = curr.older
			continue
		}
		reversedMessages[count] = curr.payload
		count++
		curr = curr.older
	}

	// reverse the array
	messages := make([]*common.Message, count)
	for i := 0; i < count; i++ {
		messages[i] = reversedMessages[count-i-1]
	}
	return messages
}
