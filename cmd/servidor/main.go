// Comando servidor levanta la API de eventos y seguimiento de envios.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MattP2001/logistica-eventos-sla/internal/almacen"
	"github.com/MattP2001/logistica-eventos-sla/internal/api"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	puerto := envOr("PORT", "8080")
	dsn := envOr("DB_PATH", "envios.db")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := almacen.Abrir(ctx, dsn)
	if err != nil {
		log.Error("abrir base", "error", err)
		os.Exit(1)
	}
	defer a.Cerrar()

	srv := &http.Server{
		Addr:              ":" + puerto,
		Handler:           api.Nuevo(a, time.Now, log).Rutas(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		apagado, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(apagado)
	}()

	log.Info("escuchando", "puerto", puerto, "db", dsn)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("servidor", "error", err)
		os.Exit(1)
	}
}

func envOr(clave, def string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return def
}
