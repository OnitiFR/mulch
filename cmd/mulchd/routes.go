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
		Auth:         server.RouteAuthPublic,
		NoProtoCheck: true,
		Handler:      controllers.PhoneController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /locked",
		Type:         server.RouteTypeCustom,
		Auth:         server.RouteAuthPublic,
		NoProtoCheck: true,
		Handler:      controllers.LockedController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /cloud-init/*",
		Type:         server.RouteTypeCustom,
		Auth:         server.RouteAuthPublic,
		NoProtoCheck: true,
		Handler:      controllers.CloudInitController,
	}, server.RouteInternal)

	app.AddRoute(&server.Route{
		Route:        "GET /env",
		Type:         server.RouteTypeCustom,
		Auth:         server.RouteAuthPublic,
		NoProtoCheck: true,
		Handler:      controllers.EnvController,
	}, server.RouteInternal)

	// API routes
	app.AddRoute(&server.Route{
		Route:   "POST /auth/challenge",
		Type:    server.RouteTypeCustom,
		Auth:    server.RouteAuthPublic,
		Handler: controllers.AuthChallengeController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /auth/token",
		Type:    server.RouteTypeCustom,
		Auth:    server.RouteAuthPublic,
		Handler: controllers.AuthTokenController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /log/history",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetLogHistoryController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /log",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.LogController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/search",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.SearchVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/config/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConfigController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/infos/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMInfosController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/load/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMLoadController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/do-actions/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMDoActionsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/console/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetVMConsoleController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.NewVMSyncController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm-async",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.NewVMAsyncController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /vm/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.ActionVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /vm/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /version",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.VersionController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /seed",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListSeedController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /seed/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetSeedStatusController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /seed/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.ActionSeedController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /backup",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListBackupsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /backup",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.UploadBackupController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /backup/expire/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.SetBackupExpireController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /backup/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.DownloadBackupController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /backup/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteBackupController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeysController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.NewKeyController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/me/comment",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetMyKeyCommentController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/right/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeyRightsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/right/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.NewKeyRightController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/right/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyRightController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/ssh/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeySSHController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/ssh/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.AddKeySSHController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/ssh/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeySSHController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/ssh-only/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.SetKeySSHOnlyController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /key/trust/list/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListKeyTrustedVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/trust/list/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.AddKeyTrustedVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /key/trust/list/*",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteKeyTrustedVMController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /key/trust/clean",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeStream,
		Handler: controllers.CleanKeyTrustedVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /sshpair",
		Auth:    server.RouteAuthFresh,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetKeyPairController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /status",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetStatusController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /state/zip",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetStateZipController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /peer",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListPeersController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.GetSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /secret/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.SetSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /secret/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.DeleteSecretController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "POST /secret-sync",
		Auth:    server.RouteAuthKey,
		Handler: controllers.SyncSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret-usage",
		Auth:    server.RouteAuthKey,
		Handler: controllers.ListSecretsUsageController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /secret-stats",
		Auth:    server.RouteAuthKey,
		Handler: controllers.SecretsStatsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /vm/with-secret/*",
		Auth:    server.RouteAuthKey,
		Handler: controllers.GetVMsUsingSecretsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "GET /greenhouse",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeCustom,
		Handler: controllers.ListGreenhouseVMsController,
	}, server.RouteAPI)

	app.AddRoute(&server.Route{
		Route:   "DELETE /greenhouse/*",
		Auth:    server.RouteAuthKey,
		Type:    server.RouteTypeStream,
		Handler: controllers.AbordGreenhouseVMController,
	}, server.RouteAPI)

}
