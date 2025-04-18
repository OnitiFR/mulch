package main

import (
	"github.com/OnitiFR/mulch/cmd/mulchd/controllers"
	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

// AddRoutes defines all API routes for the application
func AddRoutes(app *server.App) {
	// Internal routes
	app.AddRoute(&server.Route{
		Route:        "POST /phone",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.PhoneController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /locked",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.LockedController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /cloud-init/*",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.CloudInitController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /env",
		Type:         server.RouteTypeCustom,
		Public:       true,
		NoProtoCheck: true,
		Handler:      controllers.EnvController,
	}, server.RouteInternal)

	// API routes
	app.AddRoute(&server.Route{
		Route:   "GET /log/history",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetLogHistoryController,
	}, server.RouteAPI)

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
		Route:   "GET /vm/search",
		Type:    server.RouteTypeCustom,
		Handler: controllers.SearchVMsController,
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
		Route:   "GET /vm/load/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMLoadController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/do-actions/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMDoActionsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/console/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConsoleController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewVMSyncController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm-async",
		Type:    server.RouteTypeCustom,
		Handler: controllers.NewVMAsyncController,
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
		Route:   "POST /seed/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.ActionSeedController,
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
		Route:   "POST /backup/expire/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.SetBackupExpireController,
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
		Route:   "DELETE /key/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/right/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeyRightsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/right/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.NewKeyRightController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/right/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyRightController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/trust/list/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeyTrustedVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/trust/list/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.AddKeyTrustedVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/trust/list/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyTrustedVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/trust/clean",
		Type:    server.RouteTypeStream,
		Handler: controllers.CleanKeyTrustedVMsController,
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

	app.AddRoute(&server.Route{
		Route:   "GET /state/zip",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetStateZipController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /peer",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListPeersController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret/*",
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /secret/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.SetSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /secret/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /secret-sync",
		Handler: controllers.SyncSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret-usage",
		Handler: controllers.ListSecretsUsageController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret-stats",
		Handler: controllers.SecretsStatsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/with-secret/*",
		Handler: controllers.GetVMsUsingSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /greenhouse",
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListGreenhouseVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /greenhouse/*",
		Type:    server.RouteTypeStream,
		Handler: controllers.AbordGreenhouseVMController,
	}, server.RouteAPI)

}
