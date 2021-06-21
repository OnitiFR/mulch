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
	"sync/atomic"
	"time"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// request protocols
const (
	ProtoHTTP  = "http"
	ProtoHTTPS = "https"
)

// ProxyServer describe a Mulch proxy server
type ProxyServer struct {
	DomainDB    *DomainDatabase
	Log         *Log
	RequestList *RequestList
	HTTP        *http.Server
	HTTPS       *http.Server
	config      *ProxyServerParams
}

// ProxyServerParams is needed to create a ProxyServer
type ProxyServerParams struct {
	DirCache              string
	Email                 string
	ListenHTTP            string
	ListenHTTPS           string
	DirectoryURL          string
	DomainDB              *DomainDatabase
	ErrorHTMLTemplateFile string
	MulchdHTTPSDomain     string // (for mulchd)
	ChainMode             int
	ChainPSK              string
	ChainDomain           string
	ForceXForwardedFor    bool
	Log                   *Log
	RequestList           *RequestList
	Trace                 bool
	Debug                 bool
}

var contextKeyID interface{} = 1
var requestCounter uint64

// Until Go 1.11 and his reverseProxy.ErrorHandler is mainstream, let's
// have our own error generator
type errorHandlingRoundTripper struct {
	ProxyServer *ProxyServer
	Domain      *common.Domain
	Log         *Log
}

func (rt *errorHandlingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// we may have to clone http.DefaultTransport to adjust settings
	// t := http.DefaultTransport.(*http.Transport).Clone()
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
			Header:        make(http.Header),
		}, nil
	}
	return res, err
}

// NewProxyServer instanciates a new ProxyServer
func NewProxyServer(config *ProxyServerParams) *ProxyServer {
	proxy := ProxyServer{
		DomainDB:    config.DomainDB,
		Log:         config.Log,
		RequestList: config.RequestList,
		config:      config,
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
	// allow the admin to drastically lower this value. (ex: Træfik default is 3min)
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
	if host == proxy.config.MulchdHTTPSDomain && proxy.config.MulchdHTTPSDomain != "" {
		proxy.Log.Trace("hostPolicy OK for MulchdHTTPSDomain")
		return nil
	}

	if host == proxy.config.ChainDomain && proxy.config.ChainDomain != "" {
		proxy.Log.Trace("hostPolicy OK for ChainDomain")
		return nil
	}

	_, err := proxy.DomainDB.GetByName(host)
	if err == nil {
		proxy.Log.Tracef("hostPolicy OK, host '%s' found in DomainDB", host)
		return nil
	}

	return fmt.Errorf("hostPolicy ERROR, host '%s' not found in DomainDB", host)
}

// func reverseProxyErrorHandler(rw http.ResponseWriter, req *http.Request, err error) {
// 	rw.WriteHeader(http.StatusBadGateway)
// }

func (proxy *ProxyServer) serveReverseProxy(domain *common.Domain, proto string, res http.ResponseWriter, req *http.Request, fromParent bool) {
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

	// we are a parent and this request is forwarded to a child, add PSK to
	// authenticate ourself
	if proxy.config.ChainMode == ChainModeParent && domain.Chained {
		req.Header.Set(PSKHeaderName, proxy.config.ChainPSK)
	}

	if fromParent {
		// delete PSK, since our next destination is the VM itself
		req.Header.Del(PSKHeaderName)
	} else {
		// we erase this header, so it's a bit more… believable.
		req.Header.Set("X-Real-Ip", ip)
	}

	if proxy.config.ForceXForwardedFor {
		var remoteAddr = req.RemoteAddr

		// prevent httputil.ReverseProxy to set X-Forwarded-For by itself
		req.RemoteAddr = "invalid"
		if !fromParent {
			req.Header.Set("X-Forwarded-For", ip)
		}
		domain.ReverseProxy.ServeHTTP(res, req)
		req.RemoteAddr = remoteAddr // restore value for the RequestList
	} else {
		domain.ReverseProxy.ServeHTTP(res, req)
	}

}

func (proxy *ProxyServer) handleRequest(res http.ResponseWriter, req *http.Request) {
	// remove any port info from req.Host for the lookup
	parts := strings.Split(req.Host, ":")
	host := strings.ToLower(parts[0])

	if req.Header.Get(WatchDogHeaderName) != "" {
		res.Write([]byte("OK"))
		return
	}

	fromParent := false
	if proxy.config.ChainMode == ChainModeChild && proxy.config.ChainPSK == req.Header.Get(PSKHeaderName) {
		fromParent = true
	}

	proto := ProtoHTTP
	if req.TLS != nil {
		proto = ProtoHTTPS
	}

	id := atomic.AddUint64(&requestCounter, 1)

	if proxy.config.Debug {
		ctx := req.Context()
		ctx = context.WithValue(ctx, contextKeyID, id)
		req = req.WithContext(ctx)

		proxy.RequestList.AddRequest(id, req)
		defer proxy.RequestList.DeleteRequest(id)
	}

	if proxy.config.Trace {
		// User-Agent? Datetime?
		proxy.Log.Tracef("> {%d} %s %s %t %s %s %s", id, req.RemoteAddr, proto, fromParent, req.Host, req.Method, req.RequestURI)
	}

	// trust our parent, whatever protocol was used inter-proxy
	if fromParent {
		proto = req.Header.Get("X-Forwarded-Proto")
	}

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

		// see also server/vm_config.go
		// default redirect code
		status := http.StatusFound
		switch domain.RedirectCode {
		case http.StatusMovedPermanently: // 301
			status = http.StatusMovedPermanently
		case http.StatusFound: // 302
			status = http.StatusFound

		case http.StatusTemporaryRedirect: // 307
			status = http.StatusTemporaryRedirect
		case http.StatusPermanentRedirect: // 308
			status = http.StatusPermanentRedirect
		}

		http.Redirect(res, req, newURI, status)
		return
	}

	// redirect to https?
	if proto == ProtoHTTP && domain.RedirectToHTTPS {
		newURI := "https://" + req.Host + req.URL.String()
		http.Redirect(res, req, newURI, http.StatusFound)
		return
	}

	// now, do our proxy job
	proxy.serveReverseProxy(domain, proto, res, req, fromParent)
}

// RefreshReverseProxies create new (internal) ReverseProxy instances
// This function should be called when DomainDB is updated
func (proxy *ProxyServer) RefreshReverseProxies() {
	domains := proxy.DomainDB.GetDomainsNames()
	count := 0

	for _, domainName := range domains {
		domain, err := proxy.DomainDB.GetByName(domainName)
		if err != nil {
			proxy.Log.Errorf("initReverseProxies: %s", err)
			continue
		}

		if !domain.Chained {
			domain.TargetURL = fmt.Sprintf("http://%s:%d", domain.DestinationHost, domain.DestinationPort)
		}

		pURL, _ := url.Parse(domain.TargetURL)
		domain.ReverseProxy = httputil.NewSingleHostReverseProxy(pURL)

		// domain.reverseProxy.ErrorHandler = reverseProxyErrorHandler
		domain.ReverseProxy.ModifyResponse = func(resp *http.Response) (err error) {
			if proxy.config.ChainMode != ChainModeParent {
				resp.Header.Set("X-Mulch", domain.VMName)
			}

			if proxy.config.Debug {
				ctx := resp.Request.Context()
				proxy.Log.Tracef("< {%d} %d", ctx.Value(contextKeyID), resp.StatusCode)
			}

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
	err := proxy.DomainDB.Reload()
	if err != nil {
		proxy.Log.Error(err.Error())
	}
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
