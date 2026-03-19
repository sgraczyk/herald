package store

import (
	"testing"
)

func TestArchiveConversation(t *testing.T) {
	db := testDB(t)

	// Add some messages and a summary.
	for i := 0; i < 3; i++ {
		if err := db.Append(1, msg("user", "hello"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := db.SaveSummary(1, "test summary"); err != nil {
		t.Fatalf("save summary: %v", err)
	}

	// Archive.
	archived, err := db.ArchiveConversation(1)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !archived {
		t.Fatal("expected archived=true")
	}

	// History should be empty.
	count, err := db.Count(1)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 messages after archive, got %d", count)
	}

	// Summary should be empty.
	summary, err := db.GetSummary(1)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "" {
		t.Fatalf("expected empty summary after archive, got %q", summary)
	}

	// Archive should exist.
	convs, err := db.ListArchived(1)
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected 1 archived conversation, got %d", len(convs))
	}
	if len(convs[0].Messages) != 3 {
		t.Fatalf("expected 3 messages in archive, got %d", len(convs[0].Messages))
	}
	if convs[0].Summary != "test summary" {
		t.Fatalf("expected summary in archive, got %q", convs[0].Summary)
	}
}

func TestArchiveConversationEmpty(t *testing.T) {
	db := testDB(t)

	archived, err := db.ArchiveConversation(1)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived {
		t.Fatal("expected archived=false for empty chat")
	}

	convs, err := db.ListArchived(1)
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(convs) != 0 {
		t.Fatalf("expected 0 archived conversations, got %d", len(convs))
	}
}

func TestArchiveConversationPreservesMemory(t *testing.T) {
	db := testDB(t)

	// Add a memory.
	mem := Memory{Fact: "likes Go", Source: "explicit"}
	if err := db.AddMemory(1, mem); err != nil {
		t.Fatalf("add memory: %v", err)
	}

	// Add messages and archive.
	if err := db.Append(1, msg("user", "hello"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := db.ArchiveConversation(1); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Memory should still exist.
	mems, err := db.ListMemories(1)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory after archive, got %d", len(mems))
	}
	if mems[0].Fact != "likes Go" {
		t.Fatalf("unexpected memory: %+v", mems[0])
	}
}

func TestArchiveMultiple(t *testing.T) {
	db := testDB(t)

	// Create and archive two conversations.
	for i := 0; i < 2; i++ {
		if err := db.Append(1, msg("user", "hello"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
		archived, err := db.ArchiveConversation(1)
		if err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
		if !archived {
			t.Fatalf("expected archived=true on iteration %d", i)
		}
	}

	convs, err := db.ListArchived(1)
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("expected 2 archived conversations, got %d", len(convs))
	}
}

func TestListArchivedEmpty(t *testing.T) {
	db := testDB(t)

	convs, err := db.ListArchived(999)
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(convs) != 0 {
		t.Fatalf("expected 0 archived conversations, got %d", len(convs))
	}
}

func TestArchiveSeparateChats(t *testing.T) {
	db := testDB(t)

	// Add messages to two chats.
	if err := db.Append(1, msg("user", "chat1"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := db.Append(2, msg("user", "chat2"), 50); err != nil {
		t.Fatalf("append: %v", err)
	}

	// Archive chat 1 only.
	if _, err := db.ArchiveConversation(1); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// Chat 2 should be unaffected.
	count, err := db.Count(2)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 message in chat 2, got %d", count)
	}

	// Chat 1 archives should have 1, chat 2 should have 0.
	convs1, _ := db.ListArchived(1)
	convs2, _ := db.ListArchived(2)
	if len(convs1) != 1 {
		t.Fatalf("expected 1 archive for chat 1, got %d", len(convs1))
	}
	if len(convs2) != 0 {
		t.Fatalf("expected 0 archives for chat 2, got %d", len(convs2))
	}
}

func TestPruneArchivedExceedsLimit(t *testing.T) {
	db := testDB(t)

	// Archive 5 conversations.
	for i := 0; i < 5; i++ {
		if err := db.Append(1, msg("user", "hello"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := db.ArchiveConversation(1); err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
	}

	// Prune to limit 3.
	if err := db.PruneArchived(1, 3); err != nil {
		t.Fatalf("prune: %v", err)
	}

	convs, err := db.ListArchived(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(convs) != 3 {
		t.Fatalf("expected 3 archives after prune, got %d", len(convs))
	}
}

func TestPruneArchivedUnderLimit(t *testing.T) {
	db := testDB(t)

	// Archive 2 conversations.
	for i := 0; i < 2; i++ {
		if err := db.Append(1, msg("user", "hello"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := db.ArchiveConversation(1); err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
	}

	// Prune with limit 10 — should be a no-op.
	if err := db.PruneArchived(1, 10); err != nil {
		t.Fatalf("prune: %v", err)
	}

	convs, _ := db.ListArchived(1)
	if len(convs) != 2 {
		t.Fatalf("expected 2 archives (no pruning needed), got %d", len(convs))
	}
}

func TestClearArchived(t *testing.T) {
	db := testDB(t)

	// Archive 3 conversations.
	for i := 0; i < 3; i++ {
		if err := db.Append(1, msg("user", "hello"), 50); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := db.ArchiveConversation(1); err != nil {
			t.Fatalf("archive %d: %v", i, err)
		}
	}

	if err := db.ClearArchived(1); err != nil {
		t.Fatalf("clear: %v", err)
	}

	convs, err := db.ListArchived(1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(convs) != 0 {
		t.Fatalf("expected 0 archives after clear, got %d", len(convs))
	}
}

func TestClearArchivedEmpty(t *testing.T) {
	db := testDB(t)

	// Clear on a chat with no archives — should not error.
	if err := db.ClearArchived(999); err != nil {
		t.Fatalf("clear empty: %v", err)
	}
}
