package testutil

import (
	"context"
	"testing"

	"github.com/shenthark/fuel-tracker/internal/db"
)

func MustCreateEntry(t *testing.T, d *db.DB, e db.Entry) int64 {
	t.Helper()
	id, err := d.CreateEntry(context.Background(), e)
	if err != nil {
		t.Fatalf("MustCreateEntry: %v", err)
	}
	return id
}