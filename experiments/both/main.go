package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// setcap 'cap_net_bind_service=+ep' both

// domains = ['toto.com', 'dev.toto.com::8080']
// redirect_http_to_https = true (false = proxy also from tcp/80)
// redirects = [
// 	["www.toto.com", "toto.com"],
// 	["www.dev.toto.com", "dev.toto.com"],
// ]

// Until Go 1.11 and his reverseProxy.ErrorHandler is mainstream, let's
// have our own error generator
type errorHandlingRoundTripper struct {
	Tr http.RoundTripper
}

func (rt *errorHandlingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tr := http.DefaultTransport
	res, err := tr.RoundTrip(req)
	if err != nil {
		fmt.Println(err)
		body := fmt.Sprintf("Error 502 (%s)", err)
		return &http.Response{
			StatusCode:    http.StatusBadGateway,
			Body:          ioutil.NopCloser(bytes.NewBufferString(body)),
			ContentLength: int64(len(body)),
			Request:       req,
			Header:        make(http.Header, 0),
		}, nil
	}
	return res, err
}

// Route defines a route for the reverse-proxy request handler
type Route struct {
	Domain          string
	RedirectTo      string
	DestinationPort int
	RedirectToHTTPS bool
}

var routes []*Route

func getRouteByDomain(domain string) (*Route, error) {
	for _, route := range routes {
		if route.Domain == domain {
			return route, nil
		}
	}
	return nil, fmt.Errorf("domain '%s' not found", domain)
}

func hostPolicy(ctx context.Context, host string) error {
	_, err := getRouteByDomain(host)

	if err == nil {
		return nil
	}

	return fmt.Errorf("No configuration found for host '%s' ", host)
}

func reverseProxyErrorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	rw.WriteHeader(http.StatusBadGateway)
}

func serveReverseProxy(targeturl string, res http.ResponseWriter, req *http.Request) {
	url, _ := url.Parse(targeturl)

	// we should not create a new SHRP for each request!
	reverseProxy := httputil.NewSingleHostReverseProxy(url)
	// reverseProxy.ErrorHandler = reverseProxyErrorHandler
	reverseProxy.Transport = &errorHandlingRoundTripper{}

	req.URL.Host = url.Host
	req.URL.Scheme = url.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = url.Host

	reverseProxy.ServeHTTP(res, req)
}

func handleRequest(res http.ResponseWriter, req *http.Request) {
	// remove any port info from req.Host for the lookup
	parts := strings.Split(req.Host, ":")
	host := strings.ToLower(parts[0])
	fmt.Printf("%s → %s\n", req.Host, host)

	route, err := getRouteByDomain(host)
	if err != nil {
		// default route?
		fmt.Printf("woops: %s\n", err)
		return
	}

	// redirect to another URL?
	if route.RedirectTo != "" {
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		newURI := proto + "://" + route.RedirectTo + req.URL.String()
		http.Redirect(res, req, newURI, http.StatusFound)
		return
	}

	// redirect to https?
	if req.TLS == nil && route.RedirectToHTTPS == true {
		newURI := "https://" + req.Host + req.URL.String()
		http.Redirect(res, req, newURI, http.StatusFound)
		return
	}

	// now, do our proxy job
	// TODO: change localhost to VM ip, of course
	url := fmt.Sprintf("http://localhost:%d", route.DestinationPort)
	serveReverseProxy(url, res, req)
}

func initRoutes() {
	routes = append(routes, &Route{
		Domain:          "test1.cobaye1.oniti.me",
		DestinationPort: 8081,
		RedirectToHTTPS: true,
	})
	routes = append(routes, &Route{
		Domain:          "test2.cobaye1.oniti.me",
		DestinationPort: 8082,
		RedirectToHTTPS: false,
	})
	routes = append(routes, &Route{
		Domain:     "test3.cobaye1.oniti.me",
		RedirectTo: "www.perdu.com",
		// RedirectToHTTPS has no effect here
	})
}

func main() {
	initRoutes()

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Cache:      autocert.DirCache("."),
		Email:      "julien+cobaye1@oniti.fr",
		// RenewBefore: …,
		Client: &acme.Client{
			DirectoryURL: "https://acme-staging.api.letsencrypt.org/directory",
		},
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/", handleRequest)

	httpServer := &http.Server{
		Handler: manager.HTTPHandler(mux),
		Addr:    ":80",
	}

	httpsServer := &http.Server{
		Handler:   mux,
		Addr:      ":443",
		TLSConfig: &tls.Config{GetCertificate: manager.GetCertificate},
	}

	go func() {
		fmt.Printf("Starting HTTPS server on %s\n", httpsServer.Addr)
		err := httpsServer.ListenAndServeTLS("", "")
		if err != nil {
			log.Fatalf("ListendAndServeTLS: %s", err)
		}
	}()

	go func() {
		fmt.Printf("Starting HTTP server on %s\n", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil {
			log.Fatalf("ListenAndServe: %s", err)
		}
	}()

	select {}
}
