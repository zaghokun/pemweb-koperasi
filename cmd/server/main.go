package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"koperasi/internal/config"
	"koperasi/internal/database"
	"koperasi/internal/handler"
	"koperasi/internal/repository"
	"koperasi/internal/router"
	"koperasi/internal/service"
)

func main() {
	log.Println("Memulai server Koperasi Simpan Pinjam...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Konfigurasi tidak valid: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("Gagal terhubung ke database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error saat menutup database: %v", err)
		}
	}()
	log.Println("Koneksi database pool berhasil diinisialisasi (UTC, strict parameters).")

	userRepo := repository.NewUserRepository()
	sessionRepo := repository.NewSessionRepository()
	memberRepo := repository.NewMemberRepository()
	savingRepo := repository.NewSavingRepository()
	loanRepo := repository.NewLoanRepository()
	installmentRepo := repository.NewInstallmentRepository()

	authSvc := service.NewAuthService(db, userRepo, sessionRepo)
	memberSvc := service.NewMemberService(db, memberRepo, savingRepo, loanRepo)
	savingSvc := service.NewSavingService(db, savingRepo, memberRepo)
	loanSvc := service.NewLoanService(db, loanRepo, installmentRepo, memberRepo, savingRepo)

	// Tentukan lokasi template dan static
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Gagal membaca direktori kerja: %v", err)
	}
	templatesDir := filepath.Join(workDir, "web", "templates")
	staticDir := filepath.Join(workDir, "web", "static")

	h, err := handler.NewHandler(authSvc, memberSvc, savingSvc, loanSvc, cfg.CookieSecure, templatesDir)
	if err != nil {
		log.Fatalf("Gagal memuat template HTML: %v", err)
	}

	appRouter := router.New(h, authSvc, cfg.CookieSecure, staticDir)

	server := &http.Server{
		Addr:         cfg.AppAddr,
		Handler:      appRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  1 * time.Minute,
	}

	// Jalankan server di goroutine terpisah
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server siap menerima permintaan di http://%s", cfg.AppAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Menunggu sinyal shutdown OS
	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case sig := <-shutdownSig:
		log.Printf("Menerima sinyal %v, memulai graceful shutdown...", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown gagal: %v", err)
	} else {
		log.Println("Server HTTP berhasil dimatikan secara graceful.")
	}
}
