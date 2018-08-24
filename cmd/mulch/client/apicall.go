package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/Xfennec/mulch"
	"github.com/spf13/viper"
)

// API describes the basic elements to call the API
type API struct {
	ServerURL string
	Trace     bool
}

// APICall describes a call to the API
type APICall struct {
	api    *API
	Method string
	Path   string
	Args   map[string]string
}

// NewAPI create a new API instance
func NewAPI(server string, trace bool) *API {
	return &API{
		ServerURL: server,
		Trace:     trace,
	}
}

// NewCall create a new APICall
func (api *API) NewCall(method string, path string, args map[string]string) *APICall {
	return &APICall{
		api:    api,
		Method: method,
		Path:   path,
		Args:   args,
	}
}

func cleanURL(urlIn string) (string, error) {
	urlObj, err := url.Parse(urlIn)
	if err != nil {
		return urlIn, err
	}
	urlObj.Path = path.Clean(urlObj.Path)
	return urlObj.String(), nil
}

// Do the actual API call
func (call *APICall) Do() {
	method := strings.ToUpper(call.Method)

	apiURL, err := cleanURL(call.api.ServerURL + "/" + call.Path)
	if err != nil {
		log.Fatal(err)
	}

	data := url.Values{}
	for key, val := range call.Args {
		data.Add(key, val)
	}
	if call.api.Trace == true {
		data.Add("trace", "true")
	}
	data.Add("version", VERSION)

	var req *http.Request

	switch method {
	case "GET":
		finalURL := apiURL + "?" + data.Encode()
		req, err = http.NewRequest(method, finalURL, nil)
		if err != nil {
			log.Fatal(err)
		}
	case "POST":
		req, err = http.NewRequest(method, apiURL, bytes.NewBufferString(data.Encode()))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	default:
		log.Fatal(fmt.Errorf("invalid method '%s'", method))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Status code is not OK: %v (%s)", resp.StatusCode, resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var m mulch.Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		if viper.GetBool("time") {
			fmt.Printf("%s %s: %s\n", m.Time, m.Type, m.Message)
		} else {
			fmt.Printf("%s: %s\n", m.Type, m.Message)
		}
	}

}
