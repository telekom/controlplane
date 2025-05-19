package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func SignalHandler(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		cancel()

		<-sig
		os.Exit(1)
	}()
	return ctx
}
