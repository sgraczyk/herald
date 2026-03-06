package store

import (
	"fmt"
	"testing"
	"time"
)

func TestAddAndListMemories(t *testing.T) {
	db := testDB(t)

	mem := Memory{Fact: "prefers Go", Source: "explicit", Timestamp: time.Now()}
	if err := db.AddMemory(1, mem); err != nil {
		t.Fatalf("add memory: %v", err)
	}

	mems, err := db.ListMemories(1)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}
	if mems[0].Fact != "prefers Go" || mems[0].Source != "explicit" {
		t.Errorf("unexpected memory: %+v", mems[0])
	}
}

func TestListMemoriesEmpty(t *testing.T) {
	db := testDB(t)

	mems, err := db.ListMemories(999)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(mems) != 0 {
		t.Fatalf("expected 0 memories, got %d", len(mems))
	}
}

func TestRemoveMemory(t *testing.T) {
	db := testDB(t)

	db.AddMemory(1, Memory{Fact: "prefers Go over Python", Source: "explicit", Timestamp: time.Now()})
	db.AddMemory(1, Memory{Fact: "lives in Warsaw", Source: "auto", Timestamp: time.Now()})

	removed, err := db.RemoveMemory(1, "python")
	if err != nil {
		t.Fatalf("remove memory: %v", err)
	}
	if !removed {
		t.Error("expected memory to be removed")
	}

	mems, _ := db.ListMemories(1)
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory after removal, got %d", len(mems))
	}
	if mems[0].Fact != "lives in Warsaw" {
		t.Errorf("wrong memory remained: %+v", mems[0])
	}
}

func TestRemoveMemoryNoMatch(t *testing.T) {
	db := testDB(t)

	db.AddMemory(1, Memory{Fact: "prefers Go", Source: "explicit", Timestamp: time.Now()})

	removed, err := db.RemoveMemory(1, "nonexistent")
	if err != nil {
		t.Fatalf("remove memory: %v", err)
	}
	if removed {
		t.Error("expected no memory to be removed")
	}
}

func TestHasMemory(t *testing.T) {
	db := testDB(t)

	db.AddMemory(1, Memory{Fact: "prefers Go", Source: "explicit", Timestamp: time.Now()})

	found, err := db.HasMemory(1, "prefers Go")
	if err != nil {
		t.Fatalf("has memory: %v", err)
	}
	if !found {
		t.Error("expected memory to be found")
	}

	found, _ = db.HasMemory(1, "PREFERS GO")
	if !found {
		t.Error("expected case-insensitive match")
	}

	found, _ = db.HasMemory(1, "prefers Python")
	if found {
		t.Error("expected no match for different fact")
	}
}

func TestMemoryPruning(t *testing.T) {
	db := testDB(t)

	// Fill beyond maxMemories.
	for i := 0; i < maxMemories+10; i++ {
		mem := Memory{Fact: fmt.Sprintf("fact %d", i), Source: "auto", Timestamp: time.Now()}
		if err := db.AddMemory(1, mem); err != nil {
			t.Fatalf("add memory %d: %v", i, err)
		}
	}

	mems, err := db.ListMemories(1)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(mems) != maxMemories {
		t.Errorf("expected %d memories after pruning, got %d", maxMemories, len(mems))
	}

	// Oldest memories should have been pruned; newest should remain.
	if mems[len(mems)-1].Fact != fmt.Sprintf("fact %d", maxMemories+9) {
		t.Errorf("expected newest memory to be last, got %q", mems[len(mems)-1].Fact)
	}
	if mems[0].Fact != "fact 10" {
		t.Errorf("expected oldest remaining memory to be 'fact 10', got %q", mems[0].Fact)
	}
}

func TestMemoriesSeparateChats(t *testing.T) {
	db := testDB(t)

	db.AddMemory(1, Memory{Fact: "chat1 fact", Source: "explicit", Timestamp: time.Now()})
	db.AddMemory(2, Memory{Fact: "chat2 fact", Source: "auto", Timestamp: time.Now()})

	mems1, _ := db.ListMemories(1)
	mems2, _ := db.ListMemories(2)

	if len(mems1) != 1 || mems1[0].Fact != "chat1 fact" {
		t.Errorf("chat 1 unexpected: %+v", mems1)
	}
	if len(mems2) != 1 || mems2[0].Fact != "chat2 fact" {
		t.Errorf("chat 2 unexpected: %+v", mems2)
	}
}
