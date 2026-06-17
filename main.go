package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shenthark/fuel-tracker/internal/auth"
	"github.com/shenthark/fuel-tracker/internal/config"
	"github.com/shenthark/fuel-tracker/internal/db"
	"github.com/shenthark/fuel-tracker/internal/handlers"
)

//go:embed all:public
var publicFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0755); err != nil {
		log.Fatalf("mkdir db dir: %v", err)
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer database.Close()

	authSvc, err := auth.NewService(database, cfg.AdminUser, cfg.AdminPass, time.Duration(cfg.SessionLifetimeHours)*time.Hour)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	staticSub, err := fs.Sub(publicFS, "public")
	if err != nil {
		log.Fatalf("fs sub: %v", err)
	}

	h := handlers.New(database, authSvc)
	router := handlers.NewRouter(h, staticSub)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s, db=%s", cfg.Port, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}