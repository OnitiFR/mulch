package main

import (
	"fmt"
	"os"
)

// App describes an the application
type App struct {
	Config   *AppConfig
	Log      *Log
	DomainDB *DomainDatabase
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

// Run will start the app (in the foreground)
func (app *App) Run() {
}
