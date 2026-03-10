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

func TestEstimateTokens(t *testing.T) {
	tt := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},      // len=1, 1/4=0, min 1
		{"ab", 1},     // len=2, 2/4=0, min 1
		{"abc", 1},    // len=3, 3/4=0, min 1
		{"abcd", 1},   // len=4, 4/4=1
		{"abcde", 1},  // len=5, 5/4=1
		{"12345678", 2},
		{string(make([]byte, 400)), 100},
	}
	for _, tc := range tt {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(len=%d) = %d, want %d", len(tc.input), got, tc.want)
		}
	}
}

func TestListWithTokenBudget_UnderBudget(t *testing.T) {
	db := testDB(t)

	// Each message ~1 token ("hi" = len 2 / 4 = min 1).
	for i := 0; i < 5; i++ {
		if err := db.Append(1, msg("user", "hi"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	msgs, err := db.ListWithTokenBudget(1, 50, 8000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
}

func TestListWithTokenBudget_TrimsOldest(t *testing.T) {
	db := testDB(t)

	// Each message ~25 tokens (100 chars / 4).
	content := string(make([]byte, 100))
	for i := 0; i < 10; i++ {
		if err := db.Append(1, msg("user", fmt.Sprintf("%s-%d", content, i)), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Budget of 100 tokens: each msg ~25 tokens, so ~4 messages fit.
	msgs, err := db.ListWithTokenBudget(1, 50, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) > 4 {
		t.Errorf("expected at most 4 messages, got %d", len(msgs))
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message")
	}
}

func TestListWithTokenBudget_HardCapWins(t *testing.T) {
	db := testDB(t)

	for i := 0; i < 10; i++ {
		if err := db.Append(1, msg("user", "hi"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Hard cap of 3, large token budget.
	msgs, err := db.ListWithTokenBudget(1, 3, 8000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (hard cap), got %d", len(msgs))
	}
}

func TestListWithTokenBudget_OversizedSingleMessage(t *testing.T) {
	db := testDB(t)

	// One message with ~250 tokens, budget of 10.
	big := string(make([]byte, 1000))
	if err := db.Append(1, msg("user", big), 50); err != nil {
		t.Fatalf("append: %v", err)
	}

	msgs, err := db.ListWithTokenBudget(1, 50, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (never empty), got %d", len(msgs))
	}
}

func TestListWithTokenBudget_Empty(t *testing.T) {
	db := testDB(t)

	msgs, err := db.ListWithTokenBudget(999, 50, 8000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestListWithTokenBudget_Disabled(t *testing.T) {
	db := testDB(t)

	for i := 0; i < 10; i++ {
		if err := db.Append(1, msg("user", string(make([]byte, 100))), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// Budget <= 0 disables token trimming.
	msgs, err := db.ListWithTokenBudget(1, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages (budget disabled), got %d", len(msgs))
	}

	msgs, err = db.ListWithTokenBudget(1, 50, -1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages (negative budget), got %d", len(msgs))
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
