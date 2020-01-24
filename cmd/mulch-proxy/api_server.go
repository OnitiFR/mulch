package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// APIServer is used by children to contact us, the parent
type APIServer struct {
	Log    *Log
	Server *http.Server
	Muxer  *http.ServeMux
}

func (srv *APIServer) registerRoutes() {
	// very crude router, because we have very few routes
	srv.Muxer.HandleFunc("/domains", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			srv.registerDomainsController(w, r)
			return
		}

		errMsg := fmt.Sprintf("Method %s not allowed for route /domains", r.Method)
		srv.Log.Errorf("%d: %s", 405, errMsg)
		http.Error(w, errMsg, 405)
	})
}

// NewAPIServer creates and runs the API server
func NewAPIServer(config *AppConfig, cacheDir string, log *Log) (*APIServer, error) {
	srv := APIServer{
		Log:   log,
		Muxer: http.NewServeMux(),
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
	response.Write([]byte("Hi!"))
}
