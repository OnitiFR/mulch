package main

import "github.com/Xfennec/mulch/cmd/mulchd/server"
import "github.com/Xfennec/mulch/cmd/mulchd/controllers"

// AddRoutes defines all API routes for the application
func AddRoutes(app *server.App) {
	server.AddRoute(&server.Route{
		Methods:      []string{"POST"},
		Path:         "/phone",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.PhoneController,
	}, app)

	server.AddRoute(&server.Route{
		Methods: []string{"GET"},
		Path:    "/log",
		Type:    server.RouteTypeStream,
		Handler: controllers.LogController,
	}, app)

	server.AddRoute(&server.Route{
		Methods: []string{"PUT"},
		Path:    "/vm",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewVMController,
	}, app)

	server.AddRoute(&server.Route{
		Methods: []string{"GET"},
		Path:    "/version",
		Type:    server.RouteTypeCustom,
		Handler: controllers.VersionController,
	}, app)

	server.AddRoute(&server.Route{
		Methods: []string{"GET"},
		Path:    "/test",
		Type:    server.RouteTypeStream,
		Handler: controllers.TestController,
	}, app)

	server.AddRoute(&server.Route{
		Methods: []string{"POST"},
		Path:    "/test2",
		Type:    server.RouteTypeStream,
		Handler: controllers.Test2Controller,
	}, app)
}
