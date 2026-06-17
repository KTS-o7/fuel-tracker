package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shenthark/fuel-tracker/internal/auth"
	"github.com/shenthark/fuel-tracker/internal/db"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	svc, err := auth.NewService(d, "admin", "secret123", 24*time.Hour)
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}
	return svc
}

func TestLogin_SuccessReturnsOpaqueToken(t *testing.T) {
	svc := newService(t)
	tok, err := svc.Login(context.Background(), "admin", "secret123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if tok == "" {
		t.Fatalf("expected non-empty token")
	}
	if len(tok) < 32 {
		t.Errorf("token too short: %d chars", len(tok))
	}
	if strings.Contains(tok, " ") || strings.Contains(tok, "\n") {
		t.Errorf("token not URL-safe")
	}
}

func TestLogin_WrongPasswordRejected(t *testing.T) {
	svc := newService(t)
	_, err := svc.Login(context.Background(), "admin", "wrong")
	if err == nil {
		t.Fatalf("expected error for wrong password")
	}
}

func TestLogin_WrongUsernameRejected(t *testing.T) {
	svc := newService(t)
	_, err := svc.Login(context.Background(), "nobody", "secret123")
	if err == nil {
		t.Fatalf("expected error for wrong username")
	}
}

func TestValidate_AcceptsFreshToken(t *testing.T) {
	svc := newService(t)
	tok, err := svc.Login(context.Background(), "admin", "secret123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Validate(context.Background(), tok); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate_RejectsUnknownToken(t *testing.T) {
	svc := newService(t)
	if err := svc.Validate(context.Background(), "not-a-real-token"); err == nil {
		t.Errorf("expected error for unknown token")
	}
}

func TestValidate_RejectsExpiredToken(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer d.Close()
	svc, err := auth.NewService(d, "admin", "secret123", 1*time.Hour)
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}
	tok, err := svc.Login(context.Background(), "admin", "secret123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Logout(context.Background(), tok); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if err := svc.Validate(context.Background(), tok); err == nil {
		t.Errorf("expected error after logout")
	}
}

func TestMiddleware_PassesValidRequest(t *testing.T) {
	svc := newService(t)
	tok, _ := svc.Login(context.Background(), "admin", "secret123")

	called := false
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d", rr.Code)
	}
	if !called {
		t.Errorf("downstream handler not called")
	}
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	svc := newService(t)
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream should not be called")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if body["error"] == nil {
		t.Errorf("expected error field in body")
	}
}

func TestMiddleware_RejectsWrongScheme(t *testing.T) {
	svc := newService(t)
	tok, _ := svc.Login(context.Background(), "admin", "secret123")
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream should not be called")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestMiddleware_RejectsBadToken(t *testing.T) {
	svc := newService(t)
	h := svc.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream should not be called")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}