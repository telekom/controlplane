package server

import (
	"context"
	"flag"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/cmd/server/config"
	"github.com/telekom/controlplane/file-manager/pkg/backend/s3"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logLevel    string
	address     string
	configFile  string
	backendType string
)

func init() {
	flag.StringVar(&logLevel, "loglevel", "info", "log level")
	flag.StringVar(&address, "address", ":8443", "server address")
	flag.StringVar(&configFile, "configfile", "", "path to config file")
	flag.StringVar(&backendType, "backend", "", "backend type (s3)")
}

func setupLog(logLevel string) logr.Logger {
	logCfg := zap.NewProductionConfig()
	logCfg.DisableCaller = true
	logCfg.DisableStacktrace = true
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logCfg.EncoderConfig.TimeKey = "time"
	zapLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}

	logCfg.Level.SetLevel(zapLogLevel)
	zapLog := zap.Must(logCfg.Build())
	return zapr.NewLogger(zapLog)
}

func newController(ctx context.Context, cfg *config.ServerConfig) (c controller.Controller, err error) {
	if backendType != "" {
		cfg.Backend.Type = backendType
	}
	if cfg.Backend.Type == "" {
		cfg.Backend.Type = "s3"
	}

	switch cfg.Backend.Type {
	case "s3":
		fileDownloader := s3.NewS3FileDownloader()
		fileUploader := s3.NewS3FileUploader()

		c = controller.NewController(fileDownloader, fileUploader)

	default:
		return nil, errors.Errorf("unknown backend type: %s", cfg.Backend.Type)
	}

	return c, nil
}

func main() {
	//flag.Parse()
	//log := setupLog(logLevel)
	//
	//ctx := cs.SignalHandler(context.Background())
	//
	//ctrlr.SetLogger(log)
	//cfg := config.GetConfigOrDie(configFile)
	//
	//ctrl, err := newController(ctx, cfg)
	//if err != nil {
	//	log.Error(err, "failed to create controller")
	//	return
	//}
	//
	//appCfg := cs.NewAppConfig()
	//appCfg.CtxLog = &log
	//appCfg.ErrorHandler = handler.ErrorHandler
	//app := cs.NewAppWithConfig(appCfg)
	//
	//probesCtrl := cs.NewProbesController()
	//probesCtrl.Register(app, cs.ControllerOpts{})
	//
	//apiGroup := app.Group("/api")
	//handler := api.NewStrictHandler(handler.NewHandler(ctrl), nil)
	//
	////if cfg.Security.Enabled {
	////	opts := []middleware.KubernetesAuthOption{
	////		middleware.WithTrustedIssuers(cfg.Security.TrustedIssuers...),
	////		middleware.WithJWKSetURLs(cfg.Security.JWKSetURLs...),
	////		middleware.WithAccessConfig(cfg.Security.AccessConfig...),
	////	}
	////	if util.IsRunningInCluster() {
	////		log.Info("🔑 Running in cluster")
	////		opts = append(opts, middleware.WithInClusterIssuer())
	////	}
	////	apiGroup.Use(middleware.NewKubernetesAuthz(opts...))
	////}
	//
	//api.RegisterHandlersWithOptions(apiGroup, handler, api.FiberServerOptions{})
	//
	////go func() {
	////	if disableTls {
	////		fmt.Println("⚠️\tUsing HTTP instead of HTTPS. This is not secure.")
	////		if err := app.Listen(address); err != nil {
	////			log.Error(err, "failed to start server")
	////			os.Exit(1)
	////		}
	////		return
	////	}
	////
	////	ctx = logr.NewContext(ctx, log.WithName("server"))
	////	if err := serve.ServeTLS(ctx, app, address, tlsCert, tlsKey); err != nil {
	////		log.Error(err, "failed to start server")
	////		os.Exit(1)
	////	}
	////}()
	////
	////<-ctx.Done()
	////log.Info("shutting down server...")
	////if err := app.Shutdown(); err != nil {
	////	log.Error(err, "failed to shutdown server")
	////}
}
