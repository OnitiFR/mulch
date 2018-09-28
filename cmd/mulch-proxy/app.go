package main

import (
	"fmt"
	"os"
	"path"
)

// App describes an the application
type App struct {
	Config      *AppConfig
	Log         *Log
	DomainDB    *DomainDatabase
	ProxyServer *ProxyServer
}

// NewApp creates a new application
func NewApp(config *AppConfig, trace bool) (*App, error) {
	app := &App{
		Config: config,
		Log:    NewLog(trace),
	}

	app.Log.Trace("starting application")

	err := app.checkDataPath()
	if err != nil {
		return nil, err
	}

	err = app.initRouteDB()
	if err != nil {
		return nil, err
	}

	cacheDir, err := app.initCertCache()
	if err != nil {
		return nil, err
	}

	app.ProxyServer = NewProxyServer(
		cacheDir,
		app.Config.AcmeEmail,
		app.Config.HTTPAddress,
		app.Config.HTTPSAddress,
		app.Config.AcmeURL)

	return app, nil
}

func (app *App) checkDataPath() error {
	if _, err := os.Stat(app.Config.DataPath); os.IsNotExist(err) {
		return fmt.Errorf("data path (%s) does not exist", app.Config.DataPath)
	}
	return nil
}

func (app *App) initRouteDB() error {
	dbPath := app.Config.DataPath + "/mulch-proxy-domains.db"

	ddb, err := NewDomainDatabase(dbPath)
	if err != nil {
		return err
	}
	app.DomainDB = ddb

	app.Log.Infof("found %d domain(s) in database %s", app.DomainDB.Count(), dbPath)

	return nil
}

func (app *App) initCertCache() (string, error) {
	cacheDir := path.Clean(app.Config.DataPath + "/certs")

	stat, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			app.Log.Infof("%s does not exists, let's create it", cacheDir)
			errM := os.Mkdir(cacheDir, 0700)
			if errM != nil {
				return "", errM
			}
			return cacheDir, nil
		}
		return "", err
	}

	if stat.IsDir() == false {
		return "", fmt.Errorf("%s is not a directory", cacheDir)
	}

	if stat.Mode() != os.ModeDir|os.FileMode(0700) {
		fmt.Println(stat.Mode())
		return "", fmt.Errorf("%s: only the owner should be able to read/write this directory (mode 0700)", cacheDir)
	}

	return cacheDir, nil
}

// Run will start the app (in the foreground)
func (app *App) Run() {
	app.Log.Info("running proxy…")
	err := app.ProxyServer.Run()
	if err != nil {
		app.Log.Error(err.Error())
		app.Log.Info("For 'bind: permission denied' on lower ports, you may use setcap:")
		app.Log.Info("Ex: setcap 'cap_net_bind_service=+ep' mulch-proxy")
		os.Exit(99)
	}
}
