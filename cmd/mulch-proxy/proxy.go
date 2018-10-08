package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/Xfennec/mulch/common"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// ProxyServer describe a Mulch proxy server
type ProxyServer struct {
	DomainDB *DomainDatabase
	Log      *Log
	HTTP     *http.Server
	HTTPS    *http.Server
}

// Until Go 1.11 and his reverseProxy.ErrorHandler is mainstream, let's
// have our own error generator
type errorHandlingRoundTripper struct {
	ProxyServer *ProxyServer
	Domain      *common.Domain
	Log         *Log
}

func (rt *errorHandlingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tr := http.DefaultTransport
	res, err := tr.RoundTrip(req)
	if err != nil {
		rt.ProxyServer.Log.Errorf("%s: %s", rt.Domain.Name, err)
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

// NewProxyServer instanciates a new ProxyServer
func NewProxyServer(dirCache string, email string, listenHTTP string, listenHTTPS string, directoryURL string, domainDB *DomainDatabase, log *Log) *ProxyServer {
	proxy := ProxyServer{
		DomainDB: domainDB,
		Log:      log,
	}

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: proxy.hostPolicy,
		Cache:      autocert.DirCache(dirCache),
		Email:      email,
		// RenewBefore: …,
	}

	if directoryURL != "" {
		manager.Client = &acme.Client{
			DirectoryURL: directoryURL,
		}
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/", proxy.handleRequest)

	proxy.HTTP = &http.Server{
		Handler: manager.HTTPHandler(mux),
		Addr:    listenHTTP,
	}

	proxy.HTTPS = &http.Server{
		Handler:   mux,
		Addr:      listenHTTPS,
		TLSConfig: &tls.Config{GetCertificate: manager.GetCertificate},
	}

	return &proxy
}

func (proxy *ProxyServer) hostPolicy(ctx context.Context, host string) error {
	_, err := proxy.DomainDB.GetByName(host)

	if err == nil {
		return nil
	}

	return fmt.Errorf("No configuration found for host '%s' ", host)
}

// func reverseProxyErrorHandler(rw http.ResponseWriter, req *http.Request, err error) {
// 	rw.WriteHeader(http.StatusBadGateway)
// }

func (proxy *ProxyServer) serveReverseProxy(domain *common.Domain, proto string, res http.ResponseWriter, req *http.Request) {
	url, _ := url.Parse(domain.TargetURL)

	req.URL.Host = url.Host
	req.URL.Scheme = url.Scheme
	// TODO: have a look at https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Forwarded
	req.Header.Set("X-Forwarded-Proto", proto)

	// No, we don't rewrite Host header anymore, let's lie all the way to the
	// destination server (usual wp-nginx-reverse-proxy behavior)
	// req.Header.Set("X-Forwarded-Host", req.Host)
	// req.Host = url.Host

	domain.ReverseProxy.ServeHTTP(res, req)
}

func (proxy *ProxyServer) handleRequest(res http.ResponseWriter, req *http.Request) {
	// remove any port info from req.Host for the lookup
	parts := strings.Split(req.Host, ":")
	host := strings.ToLower(parts[0])
	proto := "http"
	if req.TLS != nil {
		proto = "https"
	}

	// User-Agent? Datetime?
	proxy.Log.Tracef("%s %s %s %s", req.RemoteAddr, proto, req.Host, req.RequestURI)

	domain, err := proxy.DomainDB.GetByName(host)
	if err != nil {
		// default route?
		// fmt.Printf("woops: %s\n", err)
		body := fmt.Sprintf("Error 500 (%s)", err)
		proxy.Log.Error(body)
		res.WriteHeader(500)
		res.Write([]byte(body))
		return
	}

	// redirect to another URL?
	if domain.RedirectTo != "" {
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
	proxy.serveReverseProxy(domain, proto, res, req)
}

// RefreshReverseProxies create new (internal) ReverseProxy instances
// This function should be called when DomainDB is updated
func (proxy *ProxyServer) RefreshReverseProxies() {
	domains := proxy.DomainDB.GetDomains()
	count := 0

	for _, domainName := range domains {
		domain, err := proxy.DomainDB.GetByName(domainName)
		if err != nil {
			proxy.Log.Errorf("initReverseProxies: %s", err)
			continue
		}

		domain.TargetURL = fmt.Sprintf("http://%s:%d", domain.DestinationHost, domain.DestinationPort)

		pURL, _ := url.Parse(domain.TargetURL)
		domain.ReverseProxy = httputil.NewSingleHostReverseProxy(pURL)

		// domain.reverseProxy.ErrorHandler = reverseProxyErrorHandler
		domain.ReverseProxy.Transport = &errorHandlingRoundTripper{
			ProxyServer: proxy,
			Domain:      domain,
			Log:         proxy.Log,
		}
		count++
	}
	proxy.Log.Infof("refresh: %d domain(s)", count)
}

// ReloadDomains reload domains config file
func (proxy *ProxyServer) ReloadDomains() {
	proxy.DomainDB.Reload()
	proxy.RefreshReverseProxies()
}

// Run the ProxyServer (foreground)
func (proxy *ProxyServer) Run() error {

	errorChan := make(chan error)

	go func() {
		proxy.Log.Infof("HTTPS server on %s", proxy.HTTPS.Addr)
		err := proxy.HTTPS.ListenAndServeTLS("", "")
		if err != nil {
			errorChan <- fmt.Errorf("ListendAndServeTLS: %s", err)
		}
	}()

	go func() {
		proxy.Log.Infof("HTTP server on %s", proxy.HTTP.Addr)
		err := proxy.HTTP.ListenAndServe()
		if err != nil {
			errorChan <- fmt.Errorf("ListenAndServe: %s", err)
		}
	}()

	err := <-errorChan
	return err
}
