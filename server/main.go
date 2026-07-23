package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	initLogLevel(cfg.LogLevel)
	logInfo("starting server on :%d", cfg.Port)

	srv := NewServer(cfg)

	httpServer := &http.Server{
		Addr:    ":" + itoa(cfg.Port),
		Handler: srv,
	}

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-shutdownCh
		log.Printf("received %v, initiating graceful shutdown", sig)
		srv.shutdown = true

		log.Printf("notifying %d active sessions", srv.manager.CountActive())
		srv.manager.AllSessions(func(session *Session) {
			session.mu.Lock()
			if session.Term != nil {
				wsNotify(session, wsMessage{
					Type:    "shutdown",
					Message: "Event is ending. This session will close in 30 seconds.",
				})
			}
			session.mu.Unlock()
		})

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		waitStart := time.Now()
		for {
			if srv.manager.CountActive() == 0 {
				break
			}
			if time.Since(waitStart) > 30*time.Second {
				log.Printf("force killing %d remaining sessions", srv.manager.CountActive())
				srv.manager.AllSessions(func(session *Session) {
					session.Close()
				})
				break
			}
			select {
			case <-ticker.C:
			case <-shutdownCtx.Done():
			}
		}

		log.Print("shutting down HTTP server")
		httpServer.Shutdown(shutdownCtx)
		log.Print("server stopped")
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen error: %v", err)
	}

	log.Print("clean exit")
}
