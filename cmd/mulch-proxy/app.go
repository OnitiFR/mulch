package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// App describes an the application
type App struct {
	Config      *AppConfig
	Log         *Log
	ProxyServer *ProxyServer
	APIServer   *APIServer
	PortServer  *PortServer
	Rand        *rand.Rand
}

// PSKHeaderName is the name of HTTP header for the PSK
const PSKHeaderName = "Mulch-PSK"

// WatchDogHeaderName is used for parent-to-child watchdog requests
const WatchDogHeaderName = "Mulch-Watchdog"

// RateControllerCleanupInterval is the interval for cleaning the rate controller IP entries
const RateControllerCleanupInterval = 5 * time.Minute

// NewApp creates a new application
func NewApp(config *AppConfig, trace bool, debug bool) (*App, error) {

	// -debug implies -trace
	if debug && !trace {
		trace = true
	}

	app := &App{
		Config: config,
		Log:    NewLog(trace),
		Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	app.Log.Trace("starting application")

	err := app.checkDataPath()
	if err != nil {
		return nil, err
	}

	ddb, err := app.createDomainDB()
	if err != nil {
		return nil, err
	}

	cacheDir, err := InitCertCache(app.Config.DataPath + "/certs")
	if err != nil {
		return nil, err
	}

	extraCertsDB, err := NewExtraCertsDB(
		path.Clean(app.Config.configPath+"/extra_certs.toml"),
		app.Log,
	)
	if err != nil {
		return nil, err
	}

	chainDomain := ""
	switch app.Config.ChainMode {
	case ChainModeParent:
		chainDomain = app.Config.ChainParentURL.Hostname()
	case ChainModeChild:
		chainDomain = app.Config.ChainChildURL.Hostname()
	}

	rateControllers := make(map[string]*RateController)
	for name, rcConfig := range app.Config.RateControllerConfigs {
		rateControllers[name] = NewRateController(*rcConfig)
	}

	scheduler := time.NewTicker(1 * time.Minute)
	go func() {
		for range scheduler.C {
			for _, rateController := range rateControllers {
				rateController.Clean(RateControllerCleanupInterval)
			}
		}
	}()

	app.ProxyServer = NewProxyServer(&ProxyServerParams{
		DirCache:              cacheDir,
		Email:                 app.Config.AcmeEmail,
		ListenHTTP:            app.Config.HTTPAddress,
		ListenHTTPS:           app.Config.HTTPSAddress,
		DirectoryURL:          app.Config.AcmeURL,
		DomainDB:              ddb,
		ExtraCertsDB:          extraCertsDB,
		ErrorHTMLTemplateFile: path.Clean(app.Config.configPath + "/templates/error_page.html"),
		MulchdHTTPSDomain:     app.Config.ListenHTTPSDomain,
		ChainMode:             app.Config.ChainMode,
		ChainPSK:              app.Config.ChainPSK,
		ChainDomain:           chainDomain,
		ForceXForwardedFor:    app.Config.ForceXForwardedFor,
		TrustedProxies:        app.Config.TrustedProxies,
		HaveTrustedProxies:    app.Config.HaveTrustedProxies,
		Log:                   app.Log,
		RequestList:           NewRequestList(debug),
		RateControllers:       rateControllers,
		Trace:                 trace,
		Debug:                 debug,
	})

	app.ProxyServer.RefreshReverseProxies()

	portDBFile := path.Clean(app.Config.DataPath + "/mulch-proxy-ports-v2.db")
	app.PortServer, err = NewPortServer(portDBFile, app.Log)
	if err != nil {
		return nil, err
	}

	app.initSigHUPHandler()
	app.initSigQUITHandler()

	if app.Config.ChainMode == ChainModeParent {
		app.APIServer, err = NewAPIServer(app.Config, cacheDir, app.ProxyServer, app.Log)
		if err != nil {
			return nil, err
		}
	}

	if app.Config.ChainMode == ChainModeChild {
		// if this first refresh fails, we fail.
		err = app.refreshParentDomains()
		if err != nil {
			app.Log.Error("Unable to contact parent proxy. This is a startup safety check.")
			return nil, err
		}
	}

	if app.Config.ChainMode == ChainModeParent {
		InstallWatchdog(ddb, app.Config.ChainParentURL, app.Log)
	}

	return app, nil
}

func (app *App) checkDataPath() error {
	if !common.PathExist(app.Config.DataPath) {
		return fmt.Errorf("data path (%s) does not exist", app.Config.DataPath)
	}
	lastPidFilename := path.Clean(app.Config.DataPath + "/mulch-proxy-last.pid")
	pid := os.Getpid()
	os.WriteFile(lastPidFilename, []byte(strconv.Itoa(pid)), 0644)
	return nil
}

func (app *App) createDomainDB() (*DomainDatabase, error) {
	dbPath := path.Clean(app.Config.DataPath + "/mulch-proxy-domains.db")

	autoCreate := false
	if app.Config.ChainMode == ChainModeParent {
		autoCreate = true
	}

	ddb, err := NewDomainDatabase(dbPath, autoCreate)
	if err != nil {
		return nil, err
	}

	app.Log.Infof("found %d domain(s) in database %s", ddb.Count(), dbPath)

	return ddb, nil
}

func (app *App) initSigHUPHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for sig := range c {
			if sig == syscall.SIGHUP {
				app.Log.Infof("HUP Signal, reloading domains and ports")
				app.ProxyServer.ReloadDomains()
				app.refreshDomains()
				app.PortServer.ReloadPorts()
			}
		}
	}()
}

// kill -QUIT $(pidof mulch-proxy)
func (app *App) initSigQUITHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGQUIT)

	go func() {
		for range c {
			func() { // so we can use defer
				ts := time.Now().Format("20060102-150405")
				rnd := strconv.Itoa(app.Rand.Int())
				filename := path.Clean(os.TempDir() + "/" + "mulch-proxy-" + ts + "-" + rnd + ".dump")
				file, err := os.Create(filename)
				if err != nil {
					app.Log.Errorf("unable to create %s: %s", filename, err)
					return
				}

				defer file.Close()
				writer := bufio.NewWriter(file)

				fmt.Fprintf(writer, "-- mulch-proxy %s dump (dump time: %s)\n\n", Version, ts)
				writeGoroutineStacks(writer)
				fmt.Fprintf(writer, "\n\n")
				app.ProxyServer.RequestList.Dump(writer)
				fmt.Fprintf(writer, "\n\n")
				// TODO: add a proper dump (listener list, connections per listeners, etc)
				fmt.Fprintf(writer, "port proxy: %d connection(s)\n", app.PortServer.GetTotalConnections())
				fmt.Fprintf(writer, "\n\n")
				for _, rc := range app.ProxyServer.RateControllers {
					rc.Dump(writer)
				}

				writer.Flush()
				app.Log.Infof("QUIT Signal, dumped data to %s", filename)
			}()
		}
	}()
}

func (app *App) refreshDomains() {
	if app.Config.ChainMode == ChainModeChild {
		err := app.refreshParentDomains()
		if err != nil {
			app.Log.Errorf("refreshing parent domains: %s", err)
			// TODO: use alerts like mulchd?
		}
	}
}

// contact our parent proxy and send all our routes so he can forward requests
func (app *App) refreshParentDomains() error {
	data := common.ProxyChainDomains{
		Domains:   app.ProxyServer.DomainDB.GetProxyChainDomains(),
		ForwardTo: app.Config.ChainChildURL.String(),
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}

	client := http.Client{
		Timeout: time.Duration(10 * time.Second),
	}

	req, err := http.NewRequest(
		"POST",
		app.Config.ChainParentURL.String()+"/domains",
		bytes.NewBuffer(dataJSON),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(PSKHeaderName, app.Config.ChainPSK)

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		app.Log.Info("domains successfully registered on our parent")
	} else {
		app.Log.Errorf("domains registration failed, parent returned error %d", res.StatusCode)
	}

	return nil
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
