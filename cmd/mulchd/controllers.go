package main

import (
	"fmt"
	"net"
	"time"
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
	req.SetTarget("instance-1")
	req.Stream.Info("Hello from LogController")
	time.Sleep(time.Duration(5000) * time.Millisecond)
	req.Stream.Info("Bye from LogController")
}
