package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Lets's Encrypt
// https://github.com/alash3al/httpsify/blob/master/server.go
// https://www.reddit.com/r/golang/comments/636adr/users_can_do_1line_letsencrypt_https_servers/

func serveReverseProxy(targeturl string, res http.ResponseWriter, req *http.Request) {
	url, _ := url.Parse(targeturl)

	// we should not create a new SHRP for each request!
	reverseProxy := httputil.NewSingleHostReverseProxy(url)

	// Update the headers to allow for SSL redirection
	req.URL.Host = url.Host
	req.URL.Scheme = url.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = url.Host

	reverseProxy.ServeHTTP(res, req)
}

func handleRequest(res http.ResponseWriter, req *http.Request) {
	url := "http://localhost"

	switch req.Host {
	case "test1.localhost:8080":
		url = "http://localhost:8081"
	case "test2.localhost:8080":
		url = "http://localhost:8082"
	}

	serveReverseProxy(url, res, req)
}

func main() {
	http.HandleFunc("/", handleRequest)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
