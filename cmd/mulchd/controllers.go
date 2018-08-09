package main

import (
	"fmt"
	"net"
)

// PhoneController receive "phone home" requests from instances
func PhoneController(req *Request) {
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	msg := fmt.Sprintf("phoning: id=%s, ip=%s", req.HTTP.PostFormValue("instance_id"), ip)

	// We should lookup the machine and log over there, no?
	req.App.Log.Info(msg)

	req.Response.Write([]byte("OK"))
}

// LogController sends logs to client
func LogController(req *Request) {
	// TODO: change target
	req.Stream.Info("Hello from LogController")
	// time.Sleep(time.Duration(500) * time.Millisecond)
	// req.Stream.Info("Bye from LogController")

}

/*
func phoneController0(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method.", 405)
	}

	ip := strings.Split(r.RemoteAddr, ":")
	msg := fmt.Sprintf("phoning: %s, ip=%s\n", r.PostFormValue("instance_id"), ip[0])
	// We should lookup the machine and log over there, no?
	app.Log.Info(msg)

	w.Write([]byte("OK"))
}*/

/*
func logController0(w http.ResponseWriter, r *http.Request, app *App) {
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
	// client := app.hub.Register("me", mulch.MessageNoTarget)
	client := app.Hub.Register("me", "instance-1")

	for {
		select {
		case <-cn.CloseNotify():
			client.Unregister()
			return
		// TODO: make timeout configurable
		case <-time.After(10 * time.Second):
			// Keep-alive
			m := mulch.NewMessage(mulch.MessageNoop, mulch.MessageNoTarget, "")

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
*/
