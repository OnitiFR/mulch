package mulch

// TODO: add server timestamp
type Message struct {
	// SUCCESS, ERROR, INFO, TRACE
	Type    string `json:"type"`
	Message string `json:"message"`
}
