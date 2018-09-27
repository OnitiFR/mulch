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

// setcap 'cap_net_bind_service=+ep' mulch-proxy

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

////////////////////////////////
// all this will be replaced by DomainDatabase

var domains []*Domain

func getDomainByName(name string) (*Domain, error) {
	for _, domain := range domains {
		if domain.Name == name {
			return domain, nil
		}
	}
	return nil, fmt.Errorf("domain '%s' not found", name)
}

//////////////////////////////////////

func hostPolicy(ctx context.Context, host string) error {
	_, err := getDomainByName(host)

	if err == nil {
		return nil
	}

	return fmt.Errorf("No configuration found for host '%s' ", host)
}

func reverseProxyErrorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	rw.WriteHeader(http.StatusBadGateway)
}

func serveReverseProxy(domain *Domain, res http.ResponseWriter, req *http.Request) {
	url, _ := url.Parse(domain.targetURL)

	req.URL.Host = url.Host
	req.URL.Scheme = url.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = url.Host

	domain.reverseProxy.ServeHTTP(res, req)
}

func handleRequest(res http.ResponseWriter, req *http.Request) {
	// remove any port info from req.Host for the lookup
	parts := strings.Split(req.Host, ":")
	host := strings.ToLower(parts[0])
	fmt.Printf("%s → %s\n", req.Host, host)

	domain, err := getDomainByName(host)
	if err != nil {
		// default route?
		fmt.Printf("woops: %s\n", err)
		return
	}

	// redirect to another URL?
	if domain.RedirectTo != "" {
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		newURI := proto + "://" + domain.RedirectTo + req.URL.String()
		http.Redirect(res, req, newURI, http.StatusFound)
		return
	}

	// redirect to https?
	if req.TLS == nil && domain.RedirectToHTTPS == true {
		newURI := "https://" + req.Host + req.URL.String()
		http.Redirect(res, req, newURI, http.StatusFound)
		return
	}

	// now, do our proxy job
	serveReverseProxy(domain, res, req)
}

func initReverseProxies() {
	for _, domain := range domains {
		domain.targetURL = fmt.Sprintf("http://%s:%d", domain.DestinationHost, domain.DestinationPort)

		pURL, _ := url.Parse(domain.targetURL)
		reverseProxy := httputil.NewSingleHostReverseProxy(pURL)
		// reverseProxy.ErrorHandler = reverseProxyErrorHandler
		reverseProxy.Transport = &errorHandlingRoundTripper{}

		domain.reverseProxy = reverseProxy
	}
}

func initRoutes() {
	domains = append(domains, &Domain{
		Name:            "test1.cobaye1.oniti.me",
		DestinationHost: "localhost",
		DestinationPort: 8081,
		RedirectToHTTPS: true,
	})
	domains = append(domains, &Domain{
		Name:            "test2.cobaye1.oniti.me",
		DestinationHost: "localhost",
		DestinationPort: 8082,
		RedirectToHTTPS: false,
	})
	domains = append(domains, &Domain{
		Name:       "test3.cobaye1.oniti.me",
		RedirectTo: "www.perdu.com",
		// RedirectToHTTPS has no effect here
	})
}

func main0() {
	initRoutes()
	initReverseProxies()

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
