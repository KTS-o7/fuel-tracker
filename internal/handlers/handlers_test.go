package handlers_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shenthark/fuel-tracker/internal/auth"
	"github.com/shenthark/fuel-tracker/internal/db"
	"github.com/shenthark/fuel-tracker/internal/handlers"
)

type rig struct {
	tok     string
	server  *httptest.Server
	service *auth.Service
}

func newRig(t *testing.T) *rig {
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
	tok, err := svc.Login(context.Background(), "admin", "secret123")
	if err != nil {
		t.Fatalf("svc.Login: %v", err)
	}

	h := handlers.New(d, svc)
	router := handlers.NewRouter(h, nil)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &rig{tok: tok, server: srv, service: svc}
}

func (r *rig) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, r.server.URL+path, &buf)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestEntries_CreateAndGet(t *testing.T) {
	r := newRig(t)

	create := r.do(t, "POST", "/api/entries", map[string]any{
		"date":        "2026-06-17",
		"odometer":    620.0,
		"liters":      11.0,
		"price_per_l": 110.89,
		"fuel_type":   "regular",
		"notes":       "first",
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("POST: status=%d body=%s", create.StatusCode, readBody(create))
	}
	got := decodeJSON[db.Entry](t, create)
	if got.ID <= 0 || got.Date != "2026-06-17" {
		t.Errorf("bad entry: %+v", got)
	}

	get := r.do(t, "GET", "/api/entries/"+itoa(got.ID), nil)
	if get.StatusCode != http.StatusOK {
		t.Fatalf("GET: status=%d", get.StatusCode)
	}
}

func TestEntries_CreateValidatesRequiredFields(t *testing.T) {
	r := newRig(t)

	cases := []map[string]any{
		{"date": "", "odometer": 620, "liters": 11, "price_per_l": 110},
		{"date": "2026-06-17", "odometer": -1, "liters": 11, "price_per_l": 110},
		{"date": "2026-06-17", "odometer": 620, "liters": 0, "price_per_l": 110},
		{"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": -1},
		{"date": "not-a-date", "odometer": 620, "liters": 11, "price_per_l": 110},
	}
	for i, body := range cases {
		resp := r.do(t, "POST", "/api/entries", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("case %d: expected 400, got %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestEntries_CreateValidatesFuelType(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "POST", "/api/entries", map[string]any{
		"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": 110, "fuel_type": "diesel",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad fuel_type, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEntries_ListNewestFirst(t *testing.T) {
	r := newRig(t)

	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-01", "odometer": 600, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-10", "odometer": 610, "liters": 9, "price_per_l": 110})

	resp := r.do(t, "GET", "/api/entries", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	got := decodeJSON[[]db.Entry](t, resp)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].Date != "2026-06-17" || got[2].Date != "2026-06-01" {
		t.Errorf("sort wrong: dates=%s,%s,%s", got[0].Date, got[1].Date, got[2].Date)
	}
}

func TestEntries_ListFilterByMonth(t *testing.T) {
	r := newRig(t)
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-05-30", "odometer": 580, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-01", "odometer": 600, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-30", "odometer": 620, "liters": 11, "price_per_l": 110})

	resp := r.do(t, "GET", "/api/entries?month=2026-06", nil)
	got := decodeJSON[[]db.Entry](t, resp)
	if len(got) != 2 {
		t.Errorf("expected 2 entries in June, got %d", len(got))
	}
}

func TestEntries_Update(t *testing.T) {
	r := newRig(t)
	created := decodeJSON[db.Entry](t, r.do(t, "POST", "/api/entries", map[string]any{
		"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": 110, "notes": "old",
	}))

	updated := decodeJSON[db.Entry](t, r.do(t, "PUT", "/api/entries/"+itoa(created.ID), map[string]any{
		"date": "2026-06-17", "odometer": 620, "liters": 12, "price_per_l": 110, "fuel_type": "premium", "notes": "new",
	}))
	if updated.Liters != 12 || updated.FuelType != "premium" || updated.Notes != "new" {
		t.Errorf("update not persisted: %+v", updated)
	}
}

func TestEntries_Delete(t *testing.T) {
	r := newRig(t)
	created := decodeJSON[db.Entry](t, r.do(t, "POST", "/api/entries", map[string]any{
		"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": 110,
	}))

	del := r.do(t, "DELETE", "/api/entries/"+itoa(created.ID), nil)
	if del.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", del.StatusCode)
	}
	del.Body.Close()

	get := r.do(t, "GET", "/api/entries/"+itoa(created.ID), nil)
	if get.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", get.StatusCode)
	}
}

func TestEntries_NotFoundReturns404(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/entries/99999", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEntries_RejectsInvalidID(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/entries/abc", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric id, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStats_MonthlySummary(t *testing.T) {
	r := newRig(t)
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-05-20", "odometer": 540, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-01", "odometer": 580, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-15", "odometer": 620, "liters": 11, "price_per_l": 105})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-25", "odometer": 660, "liters": 10, "price_per_l": 105})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-07-01", "odometer": 690, "liters": 9, "price_per_l": 105})

	resp := r.do(t, "GET", "/api/stats?month=2026-06", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readBody(resp))
	}
	var s handlers.Stats
	s = decodeJSON[handlers.Stats](t, resp)

	if s.Month != "2026-06" {
		t.Errorf("month: got %q", s.Month)
	}
	if s.TotalLiters < 30.99 || s.TotalLiters > 31.01 {
		t.Errorf("total_liters: got %f", s.TotalLiters)
	}
	if s.FillCount != 3 {
		t.Errorf("fill_count: got %d", s.FillCount)
	}
	if s.TotalKm < 79.99 || s.TotalKm > 80.01 {
		t.Errorf("total_km: got %f", s.TotalKm)
	}
	wantCost := 10.0*110 + 11*105 + 10*105
	if s.TotalCost < wantCost-0.01 || s.TotalCost > wantCost+0.01 {
		t.Errorf("total_cost: got %f want %f", s.TotalCost, wantCost)
	}
	if s.AvgKmpl <= 0 {
		t.Errorf("avg_kmpl: got %f", s.AvgKmpl)
	}
}

func TestStats_RequiresMonth(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/stats", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStats_BadMonthFormat(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/stats?month=not-a-month", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStats_NoEntriesReturnsZeros(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/stats?month=2026-06", nil)
	s := decodeJSON[handlers.Stats](t, resp)
	if s.TotalKm != 0 || s.TotalLiters != 0 || s.TotalCost != 0 || s.FillCount != 0 || s.AvgKmpl != 0 {
		t.Errorf("expected zeros, got %+v", s)
	}
}

func TestExport_CSVContainsAllRows(t *testing.T) {
	r := newRig(t)
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-01", "odometer": 600, "liters": 10, "price_per_l": 110})
	r.do(t, "POST", "/api/entries", map[string]any{"date": "2026-06-17", "odometer": 620, "liters": 11, "price_per_l": 110, "fuel_type": "premium", "notes": "test"})

	resp := r.do(t, "GET", "/api/export?from=2026-06-01&to=2026-06-30", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type: got %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("content-disposition: got %q", cd)
	}

	body := readBody(resp)
	records, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 rows (header + 2), got %d", len(records))
	}
	want := []string{"date", "odometer", "liters", "price_per_l", "total_cost", "fuel_type", "kmpl", "notes"}
	for i, w := range want {
		if records[0][i] != w {
			t.Errorf("header[%d]: got %q want %q", i, records[0][i], w)
		}
	}
	if records[1][0] != "2026-06-01" {
		t.Errorf("row1 date: got %q", records[1][0])
	}
	if records[1][4] != "1100.00" {
		t.Errorf("row1 total_cost: got %q want 1100.00", records[1][4])
	}
	if records[1][6] != "" {
		t.Errorf("row1 kmpl should be empty (first entry), got %q", records[1][6])
	}
	if records[2][5] != "premium" {
		t.Errorf("row2 fuel_type: got %q", records[2][5])
	}
	wantKmpl := "1.82"
	gotKmpl := records[2][6]
	if gotKmpl != wantKmpl {
		t.Errorf("row2 kmpl: got %q want %q", gotKmpl, wantKmpl)
	}
}

func TestExport_NoTokenReturns401(t *testing.T) {
	r := newRig(t)
	req, _ := http.NewRequest("GET", r.server.URL+"/api/export?from=2026-06-01&to=2026-06-30", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestExport_BadDateReturns400(t *testing.T) {
	r := newRig(t)
	resp := r.do(t, "GET", "/api/export?from=bad&to=2026-06-30", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLogin_SuccessReturnsToken(t *testing.T) {
	r := newRig(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret123"})
	resp, err := http.Post(r.server.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["token"] == nil {
		t.Errorf("no token in response")
	}
}

func TestLogin_BadCredentials401(t *testing.T) {
	r := newRig(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	resp, err := http.Post(r.server.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLogin_MissingFields400(t *testing.T) {
	r := newRig(t)
	body, _ := json.Marshal(map[string]string{"username": "admin"})
	resp, err := http.Post(r.server.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func readBody(resp *http.Response) string {
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.String()
}

func itoa(i int64) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}