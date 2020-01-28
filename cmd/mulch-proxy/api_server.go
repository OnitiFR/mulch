package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// APIServer is used by children to contact us, the parent
type APIServer struct {
	Log         *Log
	Server      *http.Server
	Muxer       *http.ServeMux
	Config      *AppConfig
	ProxyServer *ProxyServer
}

func (srv *APIServer) registerRoutes() {
	// very crude router, because we have very few routes
	srv.Muxer.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {

		if srv.checkPSK(r) == false {
			http.Error(w, "Forbidden", 403)
			return
		}

		if r.Method == "POST" {
			err := srv.registerDomainsController(w, r)
			if err == nil {
				srv.ProxyServer.RefreshReverseProxies()
			}
			return
		}

		errMsg := fmt.Sprintf("Method %s not allowed for route /domains", r.Method)
		srv.Log.Errorf("%d: %s", 405, errMsg)
		http.Error(w, errMsg, 405)
	})
}

func (srv *APIServer) checkPSK(request *http.Request) bool {
	if request.Header.Get(PSKHeaderName) == srv.Config.ChainPSK {
		return true
	}
	return false
}

// NewAPIServer creates and runs the API server
func NewAPIServer(config *AppConfig, cacheDir string, proxyServer *ProxyServer, log *Log) (*APIServer, error) {
	srv := APIServer{
		Config:      config,
		ProxyServer: proxyServer,
		Log:         log,
		Muxer:       http.NewServeMux(),
	}
	srv.registerRoutes()
	listen := ":" + config.ChainParentURL.Port()

	switch config.ChainParentURL.Scheme {
	case "http":
		srv.Server = &http.Server{
			Handler: srv.Muxer,
			Addr:    listen,
		}

		go func() {
			err := srv.Server.ListenAndServe()
			log.Errorf("ListenAndServeTLS: %s", err)
			os.Exit(99)
		}()
	case "https":
		manager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(config.ChainParentURL.Hostname()),
			Cache:      autocert.DirCache(cacheDir),
			Email:      config.AcmeEmail,
		}

		if config.AcmeURL != "" {
			manager.Client = &acme.Client{
				DirectoryURL: config.AcmeURL,
			}
		}

		srv.Server = &http.Server{
			Handler:   srv.Muxer,
			Addr:      listen,
			TLSConfig: &tls.Config{GetCertificate: manager.GetCertificate},
		}

		go func() {
			err := srv.Server.ListenAndServeTLS("", "")
			log.Errorf("ListenAndServeTLS: %s", err)
			os.Exit(99)
		}()

		srv.ScheduleSelfCalls()
	}

	log.Infof("API server at %s", config.ChainParentURL.String())

	return &srv, nil
}

func (srv *APIServer) registerDomainsController(response http.ResponseWriter, request *http.Request) error {
	var data common.ProxyChainDomains

	srv.Log.Infof("child '%s' wants to register domains", request.RemoteAddr)

	err := json.NewDecoder(request.Body).Decode(&data)
	if err != nil {
		srv.Log.Error(err.Error())
		http.Error(response, err.Error(), http.StatusBadRequest)
		return err
	}

	err = srv.ProxyServer.DomainDB.ReplaceChainedDomains(data.Domains, data.ForwardTo)
	if err != nil {
		srv.Log.Error(err.Error())
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return err
	}

	response.Header().Set("Content-Type", "application/json")
	dataJSON, err := json.Marshal("OK")
	if err != nil {
		srv.Log.Error(err.Error())
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return err
	}
	response.Write([]byte(dataJSON))
	return nil
}

func (srv *APIServer) selfCall() error {
	srv.Log.Trace("self HTTPS URL call to generate/renew certificate")

	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	res, err := client.Get(srv.Config.ChainParentURL.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// ScheduleSelfCalls call our own API HTTPS URL every 24 hour, refreshing
// the TLS certificate.
func (srv *APIServer) ScheduleSelfCalls() {
	time.Sleep(1 * time.Second)
	go func() {
		err := srv.selfCall()
		if err != nil {
			srv.Log.Warningf("unable to call our own HTTPS domain: %s", err)
		}
		time.Sleep(24 * time.Hour)
	}()
}
