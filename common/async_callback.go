package common

import (
	"net/url"
	"time"
)

// AsyncCallback are used by mulchd to notify an URL of the result
// of an asynchronous request
type AsyncCallback struct {
	Action   string
	Target   string
	Success  bool
	Error    string
	Duration time.Duration
}

// AsURLValue generate a url.Values version of an AsyncCallback
// so it can be easily POSTed
func (ac *AsyncCallback) AsURLValue() url.Values {
	success := "0"
	if ac.Success {
		success = "1"
	}

	values := url.Values{}
	values.Add("Action", ac.Action)
	values.Add("Target", ac.Target)
	values.Add("Success", success)
	values.Add("Error", ac.Error)
	values.Add("Duration", ac.Duration.String())
	return values
}
