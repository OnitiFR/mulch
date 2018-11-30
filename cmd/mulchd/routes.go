package main

import "github.com/Xfennec/mulch/cmd/mulchd/server"
import "github.com/Xfennec/mulch/cmd/mulchd/controllers"

// AddRoutes defines all API routes for the application
func AddRoutes(app *server.App) {
	app.AddRoute(&server.Route{
		Route:        "POST /phone",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.PhoneController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /log",
		Type:    server.RouteTypeStream,
		Handler: controllers.LogController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /vm",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListVMsController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /vm/config/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConfigController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /vm/infos/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMInfosController,
	})

	app.AddRoute(&server.Route{
		Route:   "POST /vm",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewVMController,
	})

	app.AddRoute(&server.Route{
		Route:   "POST /vm/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.ActionVMController,
	})

	app.AddRoute(&server.Route{
		Route:   "DELETE /vm/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteVMController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /version",
		Type:    server.RouteTypeCustom,
		Handler: controllers.VersionController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /seed",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListSeedController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /seed/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetSeedStatusController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /backup",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListBackupsController,
	})

	app.AddRoute(&server.Route{
		Route:   "POST /backup",
		Type:    server.RouteTypeStream,
		Handler: controllers.UploadBackupController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /backup/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.DownloadBackupController,
	})

	app.AddRoute(&server.Route{
		Route:   "DELETE /backup/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteBackupController,
	})

	// app.AddRoute(&server.Route{
	// 	Route:   "POST /test/*",
	// 	Type:    server.RouteTypeStream,
	// 	Handler: controllers.TestController,
	// })

	app.AddRoute(&server.Route{
		Route:   "POST /test2",
		Type:    server.RouteTypeStream,
		Handler: controllers.Test2Controller,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /key",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeysController,
	})

	app.AddRoute(&server.Route{
		Route:   "POST /key",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewKeyController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /sshpair",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetKeyPairController,
	})

	app.AddRoute(&server.Route{
		Route:   "GET /status",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetStatusController,
	})

}
