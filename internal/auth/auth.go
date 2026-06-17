package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/shenthark/fuel-tracker/internal/db"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
)

type Service struct {
	database        *db.DB
	adminUser       string
	adminPassHash   []byte
	sessionLifetime time.Duration
}

func NewService(database *db.DB, adminUser, adminPassword string, sessionLifetime time.Duration) (*Service, error) {
	if adminUser == "" || adminPassword == "" {
		return nil, errors.New("admin credentials required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &Service{
		database:        database,
		adminUser:       adminUser,
		adminPassHash:   hash,
		sessionLifetime: sessionLifetime,
	}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(s.adminUser)) != 1 {
		return "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword(s.adminPassHash, []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(token)
	expires := time.Now().Add(s.sessionLifetime)
	if err := s.database.CreateSession(ctx, hash, expires); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.database.DeleteSession(ctx, hashToken(token))
}

func (s *Service) Validate(ctx context.Context, token string) error {
	sess, err := s.database.GetSession(ctx, hashToken(token))
	if err != nil {
		return ErrInvalidToken
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.database.DeleteSession(ctx, hashToken(token))
		return ErrInvalidToken
	}
	return nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(h, prefix) {
			writeUnauthorized(w, "missing bearer token")
			return
		}
		token := strings.TrimSpace(h[len(prefix):])
		if token == "" {
			writeUnauthorized(w, "empty bearer token")
			return
		}
		if err := s.Validate(r.Context(), token); err != nil {
			writeUnauthorized(w, "invalid or expired token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}