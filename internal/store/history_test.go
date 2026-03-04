package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sgraczyk/herald/internal/provider"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func msg(role, content string) provider.Message {
	return provider.Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}

func TestAppendAndList(t *testing.T) {
	db := testDB(t)

	if err := db.Append(1, msg("user", "hello"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := db.Append(1, msg("assistant", "hi"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}

	msgs, err := db.List(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Errorf("unexpected second message: %+v", msgs[1])
	}
}

func TestListEmpty(t *testing.T) {
	db := testDB(t)

	msgs, err := db.List(999)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestCount(t *testing.T) {
	db := testDB(t)

	for i := 0; i < 5; i++ {
		if err := db.Append(1, msg("user", "msg"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	count, err := db.Count(1)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5, got %d", count)
	}
}

func TestClear(t *testing.T) {
	db := testDB(t)

	for i := 0; i < 3; i++ {
		if err := db.Append(1, msg("user", "msg"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	if err := db.Clear(1); err != nil {
		t.Fatalf("clear: %v", err)
	}

	count, err := db.Count(1)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 after clear, got %d", count)
	}
}

func TestClearNonexistent(t *testing.T) {
	db := testDB(t)

	if err := db.Clear(999); err != nil {
		t.Fatalf("clear nonexistent: %v", err)
	}
}

func TestPruning(t *testing.T) {
	db := testDB(t)
	limit := 5

	for i := 0; i < 10; i++ {
		if err := db.Append(1, msg("user", fmt.Sprintf("msg-%d", i)), limit); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	msgs, err := db.List(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(msgs) != limit {
		t.Fatalf("expected %d messages after pruning, got %d", limit, len(msgs))
	}

	// Oldest messages should be pruned; newest should remain.
	if msgs[0].Content != "msg-5" {
		t.Errorf("expected first message to be msg-5, got %s", msgs[0].Content)
	}
	if msgs[4].Content != "msg-9" {
		t.Errorf("expected last message to be msg-9, got %s", msgs[4].Content)
	}
}

func TestSeparateChats(t *testing.T) {
	db := testDB(t)

	if err := db.Append(1, msg("user", "chat1"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := db.Append(2, msg("user", "chat2"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}

	msgs1, _ := db.List(1)
	msgs2, _ := db.List(2)

	if len(msgs1) != 1 || msgs1[0].Content != "chat1" {
		t.Errorf("chat 1 unexpected: %+v", msgs1)
	}
	if len(msgs2) != 1 || msgs2[0].Content != "chat2" {
		t.Errorf("chat 2 unexpected: %+v", msgs2)
	}
}

func TestOpenCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}
}
