package web

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"
)

type runtimeContextKey struct{}

// A browser disconnect only owns approval visibility. The independent runtime
// context ends turns when the process shuts down, as the legacy Background root
// did for ordinary disconnects.
func turnLifecycleContext(request *http.Request) context.Context {
	if ctx, ok := request.Context().Value(runtimeContextKey{}).(context.Context); ok {
		return ctx
	}
	return context.Background()
}

// ServeWithContext owns the HTTP listener and active request lifetime for a
// long-lived runtime. Cancellation stops accepting requests and interrupts
// provider/tool work before waiting for bounded HTTP cleanup.
func ServeWithContext(ctx context.Context, server *Server) error {
	if ctx.Err() != nil {
		return nil
	}
	addr, err := listenAddr()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("evie serve listening on http://%s", listener.Addr())
	return serveListenerContext(ctx, listener, server.Handler())
}

func serveListenerContext(ctx context.Context, listener net.Listener, handler http.Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return context.WithValue(ctx, runtimeContextKey{}, ctx)
		},
	}
	shutdown := make(chan error, 1)
	go func() {
		<-ctx.Done()
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		err := server.Shutdown(cleanup)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
		shutdown <- err
	}()
	err := server.Serve(listener)
	cancel()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, <-shutdown)
}
