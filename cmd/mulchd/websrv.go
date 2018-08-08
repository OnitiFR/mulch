package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Xfennec/mulch"
)

func phoneController(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method.", 405)
	}

	ip := strings.Split(r.RemoteAddr, ":")
	msg := fmt.Sprintf("phoning: %s, ip=%s\n", r.PostFormValue("instance_id"), ip[0])
	// We should lookup the machine and log over there, no?
	app.log.Info(msg)

	w.Write([]byte("OK"))
}

func logController(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "GET" {
		http.Error(w, "Invalid request method.", 405)
	}

	cn, ok := w.(http.CloseNotifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	// plug ourselves into the hub
	client := app.hub.Register("me")

	for {
		select {
		case <-cn.CloseNotify():
			client.Unregister()
			return
		// TODO: make timeout configurable
		case <-time.After(10 * time.Second):
			// Keep-alive
			m := mulch.NewMessage(mulch.MessageNoop, "", "")

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-client.Messages:
			err := enc.Encode(msg)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()
			break
		}
	}
}
