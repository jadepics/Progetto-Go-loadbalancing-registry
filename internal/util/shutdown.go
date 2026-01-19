package util

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WaitForShutdown blocks until SIGINT/SIGTERM arrives and then calls fn with a context.
func WaitForShutdown(fn func(ctx context.Context)) {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	fn(ctx)
}
