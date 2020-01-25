package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// APIServer is used by children to contact us, the parent
type APIServer struct {
	Log      *Log
	Server   *http.Server
	Muxer    *http.ServeMux
	Config   *AppConfig
	DomainDB *DomainDatabase
}

func (srv *APIServer) registerRoutes() {
	// very crude router, because we have very few routes
	srv.Muxer.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {

		if srv.checkPSK(r) == false {
			http.Error(w, "Forbidden", 403)
			return
		}

		if r.Method == "POST" {
			srv.registerDomainsController(w, r)
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
func NewAPIServer(config *AppConfig, cacheDir string, domainDB *DomainDatabase, log *Log) (*APIServer, error) {
	srv := APIServer{
		Config:   config,
		DomainDB: domainDB,
		Log:      log,
		Muxer:    http.NewServeMux(),
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
			HostPolicy: autocert.HostWhitelist(config.ChainParentURL.Host),
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
	}

	log.Infof("API server at %s", config.ChainParentURL.String())

	return &srv, nil
}

func (srv *APIServer) registerDomainsController(response http.ResponseWriter, request *http.Request) {
	var data common.ProxyChainDomains

	err := json.NewDecoder(request.Body).Decode(&data)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	err = srv.DomainDB.ReplaceChainedDomains(data.Domains, data.ForwardTo)
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
	}

	response.Header().Set("Content-Type", "application/json")
	dataJSON, err := json.Marshal("OK")
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
	}
	response.Write([]byte(dataJSON))
}
