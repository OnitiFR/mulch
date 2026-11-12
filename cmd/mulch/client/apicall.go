package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/OnitiFR/mulch/common"
	"github.com/blang/semver/v4"
	"github.com/c2h5oh/datasize"
	"github.com/fatih/color"
)

// current API call VM name, if any
var currentVMName string

// API describes the basic elements to call the API
type API struct {
	ServerURL string
	APIKey    string
	SSHPubKey string // personal SSH key (replaces APIKey)
	Trace     bool
	Time      common.MessageTimestamp

	sshAuth *sshAuthState
}

// APICall describes a call to the API
type APICall struct {
	api                    *API
	Method                 string
	Path                   string
	Args                   map[string]string
	JSONCallback           func(io.Reader, http.Header)
	DestFilePath           string
	DestStream             *os.File
	DisableSpecialMessages bool
	PrintLogTarget         bool
	files                  map[string]string
	noAuth                 bool // do not send any key (auth routes)
}

// NewAPI create a new API instance
func NewAPI(server string, apiKey string, trace bool, time common.MessageTimestamp) *API {
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

// requestSignature is the signature of a sensitive request, for the
// server's challenge (see signRequest)
type requestSignature struct {
	challenge string
	signature string
}

// url returns the full URL of the call (without parameters)
func (call *APICall) url() (string, error) {
	return common.CleanURL(call.api.ServerURL + "/" + call.Path)
}

// params returns all parameters sent with the call
func (call *APICall) params() url.Values {
	data := url.Values{}
	for key, val := range call.Args {
		data.Add(key, val)
	}
	if call.api.Trace {
		data.Add("trace", "true")
	}
	return data
}

// buildRequest creates the HTTP request, using secret as the key, and
// with the request signature, if any
func (call *APICall) buildRequest(secret string, sig *requestSignature) (*http.Request, error) {
	method := strings.ToUpper(call.Method)

	apiURL, err := call.url()
	if err != nil {
		return nil, err
	}

	data := call.params()

	var req *http.Request

	switch method {
	case "GET", "DELETE":
		if len(call.files) > 0 {
			return nil, errors.New("file upload is not supported using this method")
		}
		finalURL := apiURL + "?" + data.Encode()
		req, err = http.NewRequest(method, finalURL, nil)
		if err != nil {
			return nil, err
		}
	case "POST", "PUT":
		if len(call.files) == 0 {
			// simple URL encoded form
			req, err = http.NewRequest(method, apiURL, bytes.NewBufferString(data.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		} else {
			// multipart body, with file upload

			pipeReader, pipeWriter := io.Pipe()
			multipartWriter := multipart.NewWriter(pipeWriter)

			// errors are forwarded to the HTTP client using the pipe (the
			// server may also reject the request before reading the body)
			go func() {
				for fieldname, value := range data {
					errM := multipartWriter.WriteField(fieldname, value[0])
					if errM != nil {
						pipeWriter.CloseWithError(errM)
						return
					}
				}

				for field, filename := range call.files {
					ff, errM := multipartWriter.CreateFormFile(field, path.Base(filename))
					if errM != nil {
						pipeWriter.CloseWithError(errM)
						return
					}
					file, errO := os.Open(filename)
					if errO != nil {
						pipeWriter.CloseWithError(errO)
						return
					}
					_, errC := io.Copy(ff, file)
					file.Close()
					if errC != nil {
						pipeWriter.CloseWithError(errC)
						return
					}
				}

				pipeWriter.CloseWithError(multipartWriter.Close())
			}()
			req, err = http.NewRequest(method, apiURL, pipeReader)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		}
	default:
		return nil, fmt.Errorf("apicall does not support '%s' yet", method)
	}

	if secret != "" {
		req.Header.Set("Mulch-Key", secret)
	}
	if sig != nil {
		req.Header.Set(common.SSHRequestChallengeHeader, sig.challenge)
		req.Header.Set(common.SSHRequestSignatureHeader, sig.signature)
	}
	req.Header.Set("Mulch-Version", Version)
	req.Header.Set("Mulch-Protocol", strconv.Itoa(ProtocolVersion))

	return req, nil
}

// send the request, with the given secret and signature
func (call *APICall) send(secret string, sig *requestSignature) *http.Response {
	req, err := call.buildRequest(secret, sig)
	if err != nil {
		log.Fatal(common.RemoveAPIKeyFromString(err.Error(), secret))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(common.RemoveAPIKeyFromString(err.Error(), secret))
	}
	return resp
}

// Do the actual API call
func (call *APICall) Do() {
	secret := ""
	fromCache := false
	if !call.noAuth {
		var err error
		secret, fromCache, err = call.api.getSecret()
		if err != nil {
			log.Fatalf("Error: %s", err)
		}
	}

	// the request may be sent again (once for each case):
	// - with a new session token, if the cached one was rejected
	// - signed, if the server requires it (sensitive request)
	var sig *requestSignature
	var resp *http.Response
	for {
		resp = call.send(secret, sig)
		if resp.StatusCode == http.StatusOK {
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}

		// cached session token rejected (expired, server restart, …)?
		if fromCache && resp.StatusCode == http.StatusForbidden && strings.TrimSpace(string(body)) == "invalid key" {
			call.api.clearCachedToken()
			secret, _, err = call.api.getSecret()
			if err != nil {
				log.Fatalf("Error: %s", err)
			}
			fromCache = false
			continue
		}

		// the request must be signed with our SSH key
		challenge := resp.Header.Get(common.SSHRequestChallengeHeader)
		if sig == nil && resp.StatusCode == http.StatusUnauthorized && challenge != "" && call.api.SSHPubKey != "" {
			sig, err = call.signRequest(challenge)
			if err != nil {
				log.Fatalf("Error: %s", err)
			}
			continue
		}

		log.Fatalf("Error: %s (%v)\nMessage: %s",
			resp.Status,
			resp.StatusCode,
			string(body),
		)
	}
	defer resp.Body.Close()

	mime := resp.Header.Get("Content-Type")
	cvnHeader := resp.Header.Get("Current-VM-Name")
	if cvnHeader != "" {
		currentVMName = cvnHeader
	}
	switch mime {
	case "application/x-ndjson":
		err := printJSONStream(resp.Body, call)
		if err != nil {
			log.Fatal(err)
		}
	case "text/plain", "text/plain; charset=utf-8":
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(string(body))
	case "application/json":
		if call.JSONCallback == nil {
			log.Fatalf("no JSON callback defined for %s %s", call.Method, call.Path)
		}
		call.JSONCallback(resp.Body, resp.Header)
		// return? call.callback?
	case "application/octet-stream":
		if call.DestFilePath == "" && call.DestStream == nil {
			log.Fatalf("no DestFilePath/DestStream defined for %s %s", call.Method, call.Path)
		}

		if call.DestFilePath != "" {
			err := downloadFile(call.DestFilePath, resp.Body)
			if err != nil {
				log.Fatal(err)
			}
		} else if call.DestStream != nil {
			_, err := io.Copy(call.DestStream, resp.Body)
			if err != nil {
				log.Fatal(err)
			}
		}
	default:
		log.Fatalf("unsupported content type '%s'", mime)
	}

	latestClientVersionKnownByServer := resp.Header.Get("Latest-Known-Client-Version")
	if latestClientVersionKnownByServer != "" {
		verFromServer, err1 := semver.Make(latestClientVersionKnownByServer)
		verSelf, err2 := semver.Make(Version)
		if err1 == nil && err2 == nil && verFromServer.GT(verSelf) {
			green := color.New(color.FgHiGreen).SprintFunc()
			yellow := color.New(color.FgHiYellow).SprintFunc()
			msg := fmt.Sprintf("According to the server, a client update is available: %s → %s\n", yellow(Version), green(latestClientVersionKnownByServer))
			msg = msg + "Update:\n"
			msg = msg + "    go install github.com/OnitiFR/mulch/cmd/mulch@latest\n"
			GetExitMessage().Message = msg
		}
	}
}

// TimestampShow allow to override -d flags
func (call *APICall) TimestampShow(show common.MessageTimestamp) {
	call.api.Time = show
}

func printJSONStream(body io.ReadCloser, call *APICall) error {
	var retError error

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

		// we erase any previous error code :(
		retError = m.Print(call.api.Time, call.PrintLogTarget)

		if !call.DisableSpecialMessages {
			err = dealWithSpecialMessages(m.Message)
			if err != nil {
				c := color.New(color.FgRed).SprintFunc()
				errMsg := c("Client error: ", err.Error())
				fmt.Fprintf(os.Stderr, "%s\n", errMsg)
			}
		}
	}
	return retError
}

func dealWithSpecialMessages(content string) error {
	if isVar, value := common.StringIsVariable(content, "_MULCH_OPEN_URL"); isVar {
		_, err := url.ParseRequestURI(value)
		if err != nil {
			return fmt.Errorf("invalid URL: %s", value)
		}
		openBrowser(value)
	}

	if isVar, value := common.StringIsVariable(content, "_MULCH_DOWNLOAD_FILE"); isVar {
		if currentVMName == "" {
			return errors.New("Current-VM-Name HTTP header is missing, it seems :(")
		}

		atPos := strings.Index(value, "@")
		if atPos == -1 {
			return fmt.Errorf("invalid format: %s (need user@path/file)", value)
		}
		user := value[:atPos]
		filepath := value[atPos+1:]

		err := SFTPCopy(currentVMName, user, filepath)
		if err != nil {
			return fmt.Errorf("sftp copy error: %s", err)
		}
	}

	return nil
}

func downloadFile(filename string, reader io.Reader) error {
	if common.PathExist(filename) {
		return fmt.Errorf("error: file '%s' already exists", filename)
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Printf("downloading %s…\n", filename)

	bytesWritten, err := io.Copy(file, reader)
	if err != nil {
		return err
	}

	fmt.Printf("finished, downloaded %s\n", (datasize.ByteSize(bytesWritten) * datasize.B).HR())
	return nil
}
