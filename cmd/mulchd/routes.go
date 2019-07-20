package main

import "github.com/OnitiFR/mulch/cmd/mulchd/server"
import "github.com/OnitiFR/mulch/cmd/mulchd/controllers"

// AddRoutes defines all API routes for the application
func AddRoutes(app *server.App) {
	app.AddRoute(&server.Route{
		Route:        "POST /phone",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.PhoneController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:   "GET /log",
		Type:    server.RouteTypeStream,
		Handler: controllers.LogController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/config/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConfigController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/infos/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMInfosController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/do-actions/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMDoActionsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.ActionVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /vm/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /version",
		Type:    server.RouteTypeCustom,
		Handler: controllers.VersionController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /seed",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListSeedController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /seed/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetSeedStatusController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /backup",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListBackupsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /backup",
		Type:    server.RouteTypeStream,
		Handler: controllers.UploadBackupController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /backup/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.DownloadBackupController,
	}, server.RouteAPI)
	app.AddRoute(&server.Route{
		Route:   "DELETE /backup/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteBackupController,
	}, server.RouteAPI)

	// app.AddRoute(&server.Route{
	// 	Route:   "POST /test/*",
	// 	Type:    server.RouteTypeStream,
	// 	Handler: controllers.TestController,
	// }, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /test2",
		Type:    server.RouteTypeStream,
		Handler: controllers.Test2Controller,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeysController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewKeyController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /sshpair",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetKeyPairController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /status",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetStatusController,
	}, server.RouteAPI)

}
