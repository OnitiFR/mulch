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
		if !srv.checkPSK(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
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
		srv.Log.Errorf("%d: %s", http.StatusMethodNotAllowed, errMsg)
		http.Error(w, errMsg, http.StatusMethodNotAllowed)
	})

	srv.Muxer.HandleFunc("/domains/conflicts", func(w http.ResponseWriter, r *http.Request) {
		if !srv.checkPSK(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if r.Method == "POST" {
			_ = srv.checkDomainsController(w, r)
			return
		}

		errMsg := fmt.Sprintf("Method %s not allowed for route /domains/conflicts", r.Method)
		srv.Log.Errorf("%d: %s", http.StatusMethodNotAllowed, errMsg)
		http.Error(w, errMsg, http.StatusMethodNotAllowed)
	})
}

func (srv *APIServer) checkPSK(request *http.Request) bool {
	return request.Header.Get(PSKHeaderName) == srv.Config.ChainPSK
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

func (srv *APIServer) checkDomainsController(response http.ResponseWriter, request *http.Request) error {
	var data common.ProxyChainDomains

	srv.Log.Infof("child '%s' wants to check domain conflicts", request.RemoteAddr)

	err := json.NewDecoder(request.Body).Decode(&data)
	if err != nil {
		srv.Log.Error(err.Error())
		http.Error(response, err.Error(), http.StatusBadRequest)
		return err
	}
	conflicts := srv.ProxyServer.DomainDB.GetConflictingDomains(data.Domains, data.ForwardTo)

	response.Header().Set("Content-Type", "application/json")
	dataJSON, err := json.Marshal(conflicts)
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
	res, err := client.Get("https://" + srv.Config.ChainParentURL.Hostname() + "/")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// ScheduleSelfCalls call our own API HTTPS URL every 24 hour, refreshing
// the TLS certificate.
func (srv *APIServer) ScheduleSelfCalls() {
	go func() {
		time.Sleep(1 * time.Second)
		for {
			err := srv.selfCall()
			if err != nil {
				srv.Log.Warningf("unable to call our own HTTPS domain: %s", err)
			}
			time.Sleep(24 * time.Hour)
		}
	}()
}
