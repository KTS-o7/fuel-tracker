package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shenthark/fuel-tracker/internal/config"
)

func TestLoad_RequiresAdminCredentials(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldwd)
	os.Unsetenv("ADMIN_USER")
	os.Unsetenv("ADMIN_PASSWORD")

	if _, err := config.Load(); err == nil {
		t.Errorf("expected error when admin creds missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldwd)

	os.Setenv("ADMIN_USER", "admin")
	os.Setenv("ADMIN_PASSWORD", "pw")
	defer os.Unsetenv("ADMIN_USER")
	defer os.Unsetenv("ADMIN_PASSWORD")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != "9124" {
		t.Errorf("Port: got %q", c.Port)
	}
	if c.DBPath != "data/fuel.db" {
		t.Errorf("DBPath: got %q", c.DBPath)
	}
	if c.SessionLifetimeHours != 24*30 {
		t.Errorf("SessionLifetimeHours: got %d", c.SessionLifetimeHours)
	}
}

func TestLoad_FromEnvFile(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldwd)

	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ADMIN_USER=u\nADMIN_PASSWORD=p\nPORT=8080\n"), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	os.Unsetenv("ADMIN_USER")
	os.Unsetenv("ADMIN_PASSWORD")
	os.Unsetenv("PORT")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.AdminUser != "u" || c.AdminPass != "p" || c.Port != "8080" {
		t.Errorf("got %+v", c)
	}
}