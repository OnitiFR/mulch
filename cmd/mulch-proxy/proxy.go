package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// ProxyServer describe a Mulch proxy server
type ProxyServer struct {
	DomainDB *DomainDatabase
	Log      *Log
	HTTP     *http.Server
	HTTPS    *http.Server
	config   *ProxyServerConfig
}

// ProxyServerConfig is needed to create a ProxyServer
type ProxyServerConfig struct {
	DirCache              string
	Email                 string
	ListenHTTP            string
	ListenHTTPS           string
	DirectoryURL          string
	DomainDB              *DomainDatabase
	ErrorHTMLTemplateFile string
	Log                   *Log
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
		body, errG := rt.ProxyServer.genErrorPage(502, err.Error())
		if errG != nil {
			rt.ProxyServer.Log.Errorf("Error with the error page: %s", errG)
		}
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
func NewProxyServer(config *ProxyServerConfig) *ProxyServer {
	proxy := ProxyServer{
		DomainDB: config.DomainDB,
		Log:      config.Log,
		config:   config,
	}

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: proxy.hostPolicy,
		Cache:      autocert.DirCache(config.DirCache),
		Email:      config.Email,
		// RenewBefore: …,
	}

	if config.DirectoryURL != "" {
		manager.Client = &acme.Client{
			DirectoryURL: config.DirectoryURL,
		}
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/", proxy.handleRequest)

	// We're still very gentle here, there are some legitimate "long idling request"
	// use case out there. But we should add a runtime setting somewhere to
	// allow the admin to drastically lower this value.
	IdleTimeout := 15 * time.Minute

	proxy.HTTP = &http.Server{
		Handler:     manager.HTTPHandler(mux),
		Addr:        config.ListenHTTP,
		IdleTimeout: IdleTimeout,
	}

	proxy.HTTPS = &http.Server{
		Handler:     mux,
		Addr:        config.ListenHTTPS,
		TLSConfig:   &tls.Config{GetCertificate: manager.GetCertificate},
		IdleTimeout: IdleTimeout,
	}

	return &proxy
}

func (proxy *ProxyServer) genErrorPage(code int, message string) (string, error) {
	htmlBytes, err := ioutil.ReadFile(proxy.config.ErrorHTMLTemplateFile)
	if err != nil {
		return err.Error(), err
	}
	html := string(htmlBytes)

	variables := make(map[string]interface{})
	variables["ERROR_CODE"] = strconv.Itoa(code)
	variables["ERROR_MESSAGE"] = message

	expanded := common.StringExpandVariables(html, variables)

	return expanded, nil
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

	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		ip = "invalid-" + req.RemoteAddr
	}
	// We erase this header, so it's a bit more… believable.
	req.Header.Set("X-Real-Ip", ip)

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
		body, errG := proxy.genErrorPage(500, err.Error())
		if errG != nil {
			proxy.Log.Errorf("Error with the error page: %s", errG)
		}
		proxy.Log.Errorf("Error 500 (%s)", err)
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
		domain.ReverseProxy.ModifyResponse = func(resp *http.Response) (err error) {
			resp.Header.Add("X-Mulch", domain.VMName)
			return nil
		}
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
