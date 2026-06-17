package db_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shenthark/fuel-tracker/internal/db"
	"github.com/shenthark/fuel-tracker/internal/testutil"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpen_CreatesEntriesAndSessionsTables(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	if _, err := d.ListEntries(ctx, ""); err != nil {
		t.Fatalf("entries table missing or unreadable: %v", err)
	}
	if _, err := d.GetSession(ctx, "nope"); err == nil {
		t.Fatalf("sessions table missing or GetSession not failing on missing row")
	}
}

func TestCreateEntry_AutoSetsIDAndCreatedAt(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	e := db.Entry{
		Date:       "2026-06-17",
		Odometer:   620.0,
		Liters:     11.0,
		PricePerL:  110.89,
		FuelType:   "regular",
		Notes:      "",
	}

	id, err := d.CreateEntry(ctx, e)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	got, err := d.GetEntry(ctx, id)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got.Date != e.Date || got.Odometer != e.Odometer || got.Liters != e.Liters {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
	if got.FuelType != "regular" {
		t.Errorf("fuel_type default: got %q", got.FuelType)
	}
	if got.CreatedAt == "" {
		t.Errorf("created_at not set")
	}
	if _, err := time.Parse(time.RFC3339, got.CreatedAt); err != nil {
		t.Errorf("created_at not RFC3339: %q (%v)", got.CreatedAt, err)
	}
}

func TestListEntries_NewestFirst(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-01", Odometer: 600, Liters: 10, PricePerL: 110})
	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-17", Odometer: 620, Liters: 11, PricePerL: 110})
	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-10", Odometer: 610, Liters: 9, PricePerL: 110})

	got, err := d.ListEntries(ctx, "")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].Date != "2026-06-17" || got[1].Date != "2026-06-10" || got[2].Date != "2026-06-01" {
		t.Errorf("sort order wrong: %+v", []string{got[0].Date, got[1].Date, got[2].Date})
	}
}

func TestListEntries_FilterByMonth(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-05-31", Odometer: 580, Liters: 10, PricePerL: 110})
	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-01", Odometer: 600, Liters: 10, PricePerL: 110})
	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-30", Odometer: 620, Liters: 11, PricePerL: 110})
	testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-07-01", Odometer: 640, Liters: 10, PricePerL: 110})

	got, err := d.ListEntries(ctx, "2026-06")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries in 2026-06, got %d", len(got))
	}
}

func TestUpdateEntry_PersistsChanges(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	id := testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-17", Odometer: 620, Liters: 11, PricePerL: 110, Notes: "old"})
	if err := d.UpdateEntry(ctx, id, db.Entry{Date: "2026-06-17", Odometer: 620, Liters: 12, PricePerL: 110, FuelType: "premium", Notes: "new"}); err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}
	got, err := d.GetEntry(ctx, id)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got.Liters != 12 || got.FuelType != "premium" || got.Notes != "new" {
		t.Errorf("update not persisted: %+v", got)
	}
}

func TestDeleteEntry_RemovesRow(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	id := testutil.MustCreateEntry(t, d, db.Entry{Date: "2026-06-17", Odometer: 620, Liters: 11, PricePerL: 110})
	if err := d.DeleteEntry(ctx, id); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	_, err := d.GetEntry(ctx, id)
	if err == nil {
		t.Errorf("expected error after delete, got nil")
	}
}

func TestCreateAndLookupSession(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	hash := "abc123hash"
	expires := time.Now().Add(24 * time.Hour).UTC()
	if err := d.CreateSession(ctx, hash, expires); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := d.GetSession(ctx, hash)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.TokenHash != hash {
		t.Errorf("token hash mismatch: %q", got.TokenHash)
	}
	if got.ExpiresAt.Before(time.Now()) {
		t.Errorf("session should not be expired")
	}
}

func TestGetSession_UnknownHashReturnsError(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	if _, err := d.GetSession(ctx, "nope"); err == nil {
		t.Errorf("expected error for unknown hash")
	}
}

func TestDeleteSession_RemovesRow(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	hash := "abc123hash"
	expires := time.Now().Add(24 * time.Hour).UTC()
	if err := d.CreateSession(ctx, hash, expires); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := d.DeleteSession(ctx, hash); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := d.GetSession(ctx, hash); err == nil {
		t.Errorf("expected error after delete")
	}
}