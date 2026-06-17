package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port      string
	DBPath    string
	AdminUser string
	AdminPass string
	SessionLifetimeHours int
}

func Load() (*Config, error) {
	if err := loadEnv(".env"); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	c := &Config{
		Port:                 getenv("PORT", "9124"),
		DBPath:               getenv("DB_PATH", "data/fuel.db"),
		AdminUser:            os.Getenv("ADMIN_USER"),
		AdminPass:            os.Getenv("ADMIN_PASSWORD"),
		SessionLifetimeHours: 24 * 30,
	}
	if v := os.Getenv("SESSION_LIFETIME_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid SESSION_LIFETIME_HOURS: %q", v)
		}
		c.SessionLifetimeHours = n
	}
	if c.AdminUser == "" || c.AdminPass == "" {
		return nil, fmt.Errorf("ADMIN_USER and ADMIN_PASSWORD required (set in .env)")
	}
	return c, nil
}

func loadEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}