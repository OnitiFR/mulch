package mulch

type Message struct {
	// SUCCESS, ERROR, INFO, TRACE
	Type    string `json:"type"`
	Message string `json:"message"`
}
