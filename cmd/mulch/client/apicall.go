package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/Xfennec/mulch"
	"github.com/fatih/color"
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
	files  map[string]string
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
		files:  make(map[string]string),
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

// AddFile to the request (upload)
func (call *APICall) AddFile(fieldname string, filename string) error {
	// test readability
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	call.files[fieldname] = filename
	return nil
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
	data.Add("version", Version)
	data.Add("protocol", strconv.Itoa(ProtocolVersion))

	var req *http.Request

	switch method {
	case "GET":
		if len(call.files) > 0 {
			log.Fatalf("file upload is not supported using GET method")
		}
		finalURL := apiURL + "?" + data.Encode()
		req, err = http.NewRequest(method, finalURL, nil)
		if err != nil {
			log.Fatal(err)
		}
	case "POST", "PUT":
		if len(call.files) == 0 {
			// simple URL encoded form
			req, err = http.NewRequest(method, apiURL, bytes.NewBufferString(data.Encode()))
			if err != nil {
				log.Fatal(err)
			}
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		} else {
			// multipart body, with file upload
			var buf bytes.Buffer
			multipartWriter := multipart.NewWriter(&buf)

			// range call.files
			for field, filename := range call.files {
				ff, errM := multipartWriter.CreateFormFile(field, path.Base(filename))
				if err != nil {
					log.Fatal(errM)
				}
				file, errM := os.Open(filename)
				if err != nil {
					log.Fatal(errM)
				}
				defer file.Close()
				if _, err = io.Copy(ff, file); err != nil {
					log.Fatal(err)
				}
			}
			for fieldname, value := range data {
				errM := multipartWriter.WriteField(fieldname, value[0])
				if errM != nil {
					log.Fatal(errM)
				}
			}
			err = multipartWriter.Close()
			if err != nil {
				log.Fatal(err)
			}

			req, err = http.NewRequest(method, apiURL, &buf)
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		}
	default:
		log.Fatal(fmt.Errorf("apicall does not support '%s' yet", method))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		log.Fatalf("Status code is not OK: %v (%s)\n%s",
			resp.StatusCode,
			resp.Status,
			string(body),
		)
	}

	mime := resp.Header.Get("Content-Type")

	switch mime {
	case "application/x-ndjson":
		printJSONStream(resp.Body)
	case "text/plain":
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(string(body))
	default:
		log.Fatalf("unsupported content type '%s'", mime)
	}
}

func printJSONStream(body io.ReadCloser) {
	dec := json.NewDecoder(body)
	for {
		var m mulch.Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if m.Type == mulch.MessageNoop {
			continue
		}

		// the longest types are 7 chars wide
		mtype := fmt.Sprintf("% -7s", m.Type)
		content := m.Message

		switch m.Type {
		case mulch.MessageTrace:
			c := color.New(color.FgWhite).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case mulch.MessageInfo:
		case mulch.MessageWarning:
			c := color.New(color.FgYellow).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case mulch.MessageError:
			c := color.New(color.FgRed).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case mulch.MessageFailure:
			c := color.New(color.FgHiRed).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case mulch.MessageSuccess:
			c := color.New(color.FgHiGreen).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		}

		time := ""
		if viper.GetBool("time") {
			time = m.Time.Format("15:04:05") + " "
		}
		fmt.Printf("%s%s: %s\n", time, mtype, content)
	}
}
