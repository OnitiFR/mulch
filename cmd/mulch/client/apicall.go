package client

import (
	"bytes"
	"encoding/json"
	"errors"
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

	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"github.com/fatih/color"
)

// API describes the basic elements to call the API
type API struct {
	ServerURL string
	APIKey    string
	Trace     bool
	Time      bool
}

// APICall describes a call to the API
type APICall struct {
	api          *API
	Method       string
	Path         string
	Args         map[string]string
	JSONCallback func(io.Reader)
	DestFilePath string
	files        map[string]string
}

// NewAPI create a new API instance
func NewAPI(server string, apiKey string, trace bool, time bool) *API {
	return &API{
		ServerURL: server,
		APIKey:    apiKey,
		Trace:     trace,
		Time:      time,
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
	data.Add("key", call.api.APIKey)
	data.Add("version", Version)
	data.Add("protocol", strconv.Itoa(ProtocolVersion))

	var req *http.Request

	switch method {
	case "GET", "DELETE":
		if len(call.files) > 0 {
			log.Fatal("file upload is not supported using this method")
		}
		finalURL := apiURL + "?" + data.Encode()
		req, err = http.NewRequest(method, finalURL, nil)
		if err != nil {
			log.Fatal(removeAPIKeyFromString(err.Error(), call.api.APIKey))
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
		log.Fatalf("apicall does not support '%s' yet", method)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(removeAPIKeyFromString(err.Error(), call.api.APIKey))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		log.Fatalf("Error: %s (%v)\nMessage: %s",
			resp.Status,
			resp.StatusCode,
			string(body),
		)
	}

	mime := resp.Header.Get("Content-Type")

	switch mime {
	case "application/x-ndjson":
		err := printJSONStream(resp.Body, call)
		if err != nil {
			log.Fatal(err)
		}
	case "text/plain":
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(string(body))
	case "application/json":
		if call.JSONCallback == nil {
			log.Fatalf("no JSON callback defined for %s %s", call.Method, call.Path)
		}
		call.JSONCallback(resp.Body)
		// return? call.callback?
	case "application/octet-stream":
		if call.DestFilePath == "" {
			log.Fatalf("no DestFilePath defined for %s %s", call.Method, call.Path)
		}
		downloadFile(call.DestFilePath, resp.Body)
	default:
		log.Fatalf("unsupported content type '%s'", mime)
	}
}

func removeAPIKeyFromString(in string, key string) string {
	if key == "" {
		return in
	}
	return strings.Replace(in, key, "xxx", -1)
}

func printJSONStream(body io.ReadCloser, call *APICall) error {
	var retError error
	retError = nil

	dec := json.NewDecoder(body)
	for {
		var m common.Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if m.Type == common.MessageNoop {
			continue
		}

		// the longest types are 7 chars wide
		mtype := fmt.Sprintf("% -7s", m.Type)
		content := m.Message

		switch m.Type {
		case common.MessageTrace:
			c := color.New(color.FgWhite).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case common.MessageInfo:
		case common.MessageWarning:
			c := color.New(color.FgYellow).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case common.MessageError:
			c := color.New(color.FgRed).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case common.MessageFailure:
			retError = errors.New("Exiting with failure status due to previous errors")
			c := color.New(color.FgHiRed).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		case common.MessageSuccess:
			c := color.New(color.FgHiGreen).SprintFunc()
			content = c(content)
			mtype = c(mtype)
		}

		time := ""
		if call.api.Time {
			time = m.Time.Format("15:04:05") + " "
		}
		fmt.Printf("%s%s: %s\n", time, mtype, content)
	}
	return retError
}

func downloadFile(filename string, reader io.Reader) {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("downloading %s…\n", filename)

	bytesWritten, err := io.Copy(file, reader)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("finished, downloaded %s\n", (datasize.ByteSize(bytesWritten) * datasize.B).HR())
}
