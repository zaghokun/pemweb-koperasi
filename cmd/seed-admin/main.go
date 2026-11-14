package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"koperasi/internal/config"
	"koperasi/internal/database"
	"koperasi/internal/domain"
	"koperasi/internal/repository"
)

func main() {
	usernameFlag := flag.String("username", "", "Username pengurus (unik, huruf kecil)")
	nameFlag := flag.String("name", "", "Nama lengkap pengurus")
	passwordFlag := flag.String("password", "", "Password pengurus")
	flag.Parse()

	if *usernameFlag == "" || *nameFlag == "" || *passwordFlag == "" {
		log.Fatal("Penggunaan: go run ./cmd/seed-admin -username <username> -name <nama> -password <password>")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Konfigurasi tidak valid: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("Koneksi database gagal: %v", err)
	}
	defer db.Close()

	userRepo := repository.NewUserRepository()

	// Periksa apakah username sudah terdaftar
	existing, err := userRepo.GetByUsername(ctx, db, *usernameFlag)
	if err == nil && existing != nil {
		log.Fatalf("Pengurus dengan username '%s' sudah terdaftar (ID: %d)", *usernameFlag, existing.ID)
	}

	pwHash, err := bcrypt.GenerateFromPassword([]byte(*passwordFlag), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Gagal melakukan hash password: %v", err)
	}

	newUser := &domain.User{
		Name:         strings.TrimSpace(*nameFlag),
		Username:     strings.ToLower(strings.TrimSpace(*usernameFlag)),
		PasswordHash: string(pwHash),
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}

	if err := userRepo.Create(ctx, db, newUser); err != nil {
		log.Fatalf("Gagal menyimpan akun pengurus: %v", err)
	}

	fmt.Printf("Akun pengurus berhasil dibuat:\n- ID: %d\n- Nama: %s\n- Username: %s\n",
		newUser.ID, newUser.Name, newUser.Username)
}
