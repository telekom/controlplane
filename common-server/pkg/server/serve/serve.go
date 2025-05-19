package serve

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

func ServeTLS(ctx context.Context, app *fiber.App, addr, certFile, keyFile string) error {
	cw, err := certwatcher.New(certFile, keyFile)
	if err != nil {
		return err
	}
	cw.WithWatchInterval(30 * time.Second)
	go func() {
		if err := cw.Start(ctx); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to start certwatcher")
			return
		}
	}()

	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return errors.Wrap(err, "failed to listen")
	}
	ln = tls.NewListener(ln, &tls.Config{
		GetCertificate: cw.GetCertificate,
		MinVersion:     tls.VersionTLS13,
	})

	return app.Listener(ln)
}
