package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"libvirt.org/go/libvirt"
)

type PeerCall struct {
	Peer              ConfigPeer
	Method            string
	Path              string
	Args              map[string]string
	UploadVolume      *PeerCallLibvirtFile
	UploadString      *PeerCallStringFile
	TextCallback      func(body []byte) error
	HTTPErrorCallback func(code int, body []byte, httpError error) error
	MessageCallback   func(m *common.Message) error

	Log     *Log
	Libvirt *Libvirt
}

type PeerCallLibvirtFile struct {
	Name string
	As   string
	Pool *libvirt.StoragePool
}

type PeerCallStringFile struct {
	FieldName string
	FileName  string
	Content   string
}

// Do a call to a peer (with detailed error messages)
func (call *PeerCall) Do() error {
	err := call.do()
	if err != nil {
		path := strings.TrimPrefix(call.Path, "/")
		parts := strings.Split(path, "/")
		entity := parts[0]

		if _, actionExists := call.Args["action"]; entity == "vm" && actionExists {
			entity = fmt.Sprintf("%s %s", call.Args["action"], entity)
		}

		return fmt.Errorf("call '%s' to %s failed: %s", entity, call.Peer.Name, err)
	}
	return nil
}

// do the actual call
func (call *PeerCall) do() error {
	method := strings.ToUpper(call.Method)

	apiURL, err := common.CleanURL(call.Peer.URL + "/" + call.Path)
	if err != nil {
		return err
	}

	data := url.Values{}
	for key, val := range call.Args {
		data.Add(key, val)
	}

	// data.Add("trace", "true")

	var req *http.Request
	var uploadErrChan chan error

	switch method {
	case "GET", "DELETE":
		finalURL := apiURL + "?" + data.Encode()
		req, err = http.NewRequest(method, finalURL, nil)
		if err != nil {
			msg := common.RemoveAPIKeyFromString(err.Error(), call.Peer.Key)
			return errors.New(msg)
		}
	case "POST", "PUT":
		// multipart body, with file upload (from string)
		if call.UploadString != nil {
			strFile := call.UploadString

			uploadErrChan = make(chan error, 1)
			pipeReader, pipeWriter := io.Pipe()
			multipartWriter := multipart.NewWriter(pipeWriter)

			go func() {
				defer pipeWriter.Close()

				for fieldname, value := range data {
					errM := multipartWriter.WriteField(fieldname, value[0])
					if errM != nil {
						uploadErrChan <- errM
					}
				}

				ff, errM := multipartWriter.CreateFormFile(strFile.FieldName, strFile.FileName)
				if errM != nil {
					uploadErrChan <- errM
				}

				ff.Write([]byte(strFile.Content))

				errM = multipartWriter.Close()
				if errM != nil {
					uploadErrChan <- errM
				}

				close(uploadErrChan)
			}()

			req, err = http.NewRequest(method, apiURL, pipeReader)
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		} else if call.UploadVolume != nil {
			// multipart body, with volume upload
			lvFile := call.UploadVolume
			if lvFile.As == "" {
				lvFile.As = lvFile.Name
			}

			uploadErrChan = make(chan error, 1)
			pipeReader, pipeWriter := io.Pipe()
			multipartWriter := multipart.NewWriter(pipeWriter)

			go func() {
				defer pipeWriter.Close()

				for fieldname, value := range data {
					errM := multipartWriter.WriteField(fieldname, value[0])
					if errM != nil {
						uploadErrChan <- errM
					}
				}

				ff, errM := multipartWriter.CreateFormFile("file", lvFile.As)
				if errM != nil {
					uploadErrChan <- errM
				}

				writeCloser := &common.FakeWriteCloser{Writer: ff}

				vd, errM := call.Libvirt.VolumeDownloadToWriter(lvFile.Name, lvFile.Pool, writeCloser)
				if errM != nil {
					uploadErrChan <- errM
				}

				call.Log.Infof("uploading %s to %s", lvFile.Name, call.Peer.Name)

				bytesWritten, errM := vd.Copy()
				if errM != nil {
					uploadErrChan <- errM
				}

				errM = multipartWriter.Close()
				if errM != nil {
					uploadErrChan <- errM
				}

				call.Log.Infof("uploaded %s (%s)", lvFile.Name, (datasize.ByteSize(bytesWritten) * datasize.B).HR())
				close(uploadErrChan)
			}()

			req, err = http.NewRequest(method, apiURL, pipeReader)
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
		} else {
			// simple URL encoded form

			req, err = http.NewRequest(method, apiURL, bytes.NewBufferString(data.Encode()))
			if err != nil {
				return err
			}
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		}
	default:
		return fmt.Errorf("apicall does not support '%s' yet", method)
	}

	req.Header.Set("Mulch-Key", call.Peer.Key)
	req.Header.Set("Mulch-Version", Version)
	req.Header.Set("Mulch-Protocol", strconv.Itoa(ProtocolVersion))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		msg := common.RemoveAPIKeyFromString(err.Error(), call.Peer.Key)
		return errors.New(msg)
	}
	defer resp.Body.Close()

	// read any upload error
	if uploadErrChan != nil {
		err = <-uploadErrChan
		if err != nil {
			return err
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		httpError := fmt.Errorf("Error: %s (%v)\nMessage: %s",
			resp.Status,
			resp.StatusCode,
			string(body),
		)

		if call.HTTPErrorCallback != nil {
			return call.HTTPErrorCallback(resp.StatusCode, body, httpError)
		}

		return httpError
	}

	mime := resp.Header.Get("Content-Type")
	// cvnHeader := resp.Header.Get("Current-VM-Name")
	switch mime {
	case "application/x-ndjson":
		err := decodeJSONStream(resp.Body, call)
		if err != nil {
			return err
		}
	case "text/plain", "text/plain; charset=utf-8":
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if call.TextCallback == nil {
			return errors.New("unsupported text plain response")
		} else {
			err := call.TextCallback(body)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported content type '%s'", mime)
	}

	return nil
}

func decodeJSONStream(body io.ReadCloser, call *PeerCall) error {
	var retError error

	dec := json.NewDecoder(body)
	for {
		var m common.Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if m.Type == common.MessageNoop {
			continue
		}

		var mPrefixed = m
		mPrefixed.Message = "<" + call.Peer.Name + "> " + m.Message
		// downplay success messages (helping user's readability)
		if mPrefixed.Type == common.MessageSuccess {
			mPrefixed.Type = common.MessageInfo
		}
		call.Log.Log(&mPrefixed)

		if call.MessageCallback != nil {
			err := call.MessageCallback(&m)
			if err != nil {
				return err
			}
		}

		if m.Type == common.MessageFailure {
			retError = fmt.Errorf("%s returned an error", call.Peer.Name)
		}

	}
	return retError
}
