package telegram

import (
	"testing"

	"github.com/sgraczyk/herald/internal/hub"
)

func TestNewEmptyAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, nil)
	if err == nil {
		t.Fatal("expected error for nil allowedUserIDs, got nil")
	}
}

func TestNewEmptySliceAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, []int64{})
	if err == nil {
		t.Fatal("expected error for empty allowedUserIDs, got nil")
	}
}

func TestNewZeroOnlyAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, []int64{0, 0})
	if err == nil {
		t.Fatal("expected error when all IDs are zero, got nil")
	}
}

func TestNewZeroFilteredFromAllowedIDs(t *testing.T) {
	h := hub.New()
	// This will fail at bot.New because "test-token" is not a real token,
	// but it should NOT fail with the "no valid allowed user IDs" error.
	_, err := New("test-token", h, []int64{0, 12345})
	if err == nil {
		return // bot.New succeeded (unlikely with fake token, but acceptable)
	}
	if err.Error() == "no valid allowed user IDs configured" {
		t.Fatal("zero ID was not filtered; valid ID 12345 should have been kept")
	}
}

func TestAllowedIDsMap(t *testing.T) {
	a := &Adapter{
		allowedIDs: map[int64]bool{111: true, 222: true},
	}

	if !a.allowedIDs[111] {
		t.Error("expected user 111 to be allowed")
	}
	if !a.allowedIDs[222] {
		t.Error("expected user 222 to be allowed")
	}
	if a.allowedIDs[999] {
		t.Error("expected user 999 to be rejected")
	}
}
