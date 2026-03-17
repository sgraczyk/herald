package store

import "testing"

func TestSaveSummaryAndGetSummary(t *testing.T) {
	db := testDB(t)

	if err := db.SaveSummary(1, "User discussed Go projects."); err != nil {
		t.Fatalf("save summary: %v", err)
	}

	got, err := db.GetSummary(1)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got != "User discussed Go projects." {
		t.Errorf("expected saved summary, got %q", got)
	}
}

func TestGetSummaryNonexistent(t *testing.T) {
	db := testDB(t)

	got, err := db.GetSummary(999)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for nonexistent chat, got %q", got)
	}
}

func TestSaveSummaryOverwrites(t *testing.T) {
	db := testDB(t)

	if err := db.SaveSummary(1, "first"); err != nil {
		t.Fatalf("save summary: %v", err)
	}
	if err := db.SaveSummary(1, "second"); err != nil {
		t.Fatalf("save summary: %v", err)
	}

	got, err := db.GetSummary(1)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got != "second" {
		t.Errorf("expected overwritten summary %q, got %q", "second", got)
	}
}
