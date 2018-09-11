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
		Route:   "GET /vm/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConfigController,
	})

	app.AddRoute(&server.Route{
		Route:   "PUT /vm",
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
}
