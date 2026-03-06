package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Memory represents a stored fact about the user.
type Memory struct {
	Fact      string    `json:"fact"`
	Source    string    `json:"source"` // "explicit" or "auto"
	Timestamp time.Time `json:"timestamp"`
}

// AddMemory stores a fact for a chat.
func (d *DB) AddMemory(chatID int64, mem Memory) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		memories := tx.Bucket(memoriesBucket)
		chat, err := memories.CreateBucketIfNotExists(chatBucketKey(chatID))
		if err != nil {
			return fmt.Errorf("create memory bucket: %w", err)
		}

		seq, err := chat.NextSequence()
		if err != nil {
			return fmt.Errorf("next sequence: %w", err)
		}

		data, err := json.Marshal(mem)
		if err != nil {
			return fmt.Errorf("marshal memory: %w", err)
		}

		return chat.Put(uint64Key(seq), data)
	})
}

// ListMemories returns all memories for a chat.
func (d *DB) ListMemories(chatID int64) ([]Memory, error) {
	var mems []Memory

	err := d.bolt.View(func(tx *bolt.Tx) error {
		memories := tx.Bucket(memoriesBucket)
		chat := memories.Bucket(chatBucketKey(chatID))
		if chat == nil {
			return nil
		}

		return chat.ForEach(func(k, v []byte) error {
			var mem Memory
			if err := json.Unmarshal(v, &mem); err != nil {
				return fmt.Errorf("unmarshal memory: %w", err)
			}
			mems = append(mems, mem)
			return nil
		})
	})

	return mems, err
}

// RemoveMemory deletes the first memory whose fact contains the given
// substring (case-insensitive). Returns true if a memory was removed.
func (d *DB) RemoveMemory(chatID int64, substring string) (bool, error) {
	removed := false
	lower := strings.ToLower(substring)

	err := d.bolt.Update(func(tx *bolt.Tx) error {
		memories := tx.Bucket(memoriesBucket)
		chat := memories.Bucket(chatBucketKey(chatID))
		if chat == nil {
			return nil
		}

		c := chat.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var mem Memory
			if err := json.Unmarshal(v, &mem); err != nil {
				return fmt.Errorf("unmarshal memory: %w", err)
			}
			if strings.Contains(strings.ToLower(mem.Fact), lower) {
				if err := chat.Delete(k); err != nil {
					return fmt.Errorf("delete memory: %w", err)
				}
				removed = true
				return nil
			}
		}
		return nil
	})

	return removed, err
}

// HasMemory returns true if a memory with the exact fact (case-insensitive)
// already exists for the chat.
func (d *DB) HasMemory(chatID int64, fact string) (bool, error) {
	found := false
	lower := strings.ToLower(fact)

	err := d.bolt.View(func(tx *bolt.Tx) error {
		memories := tx.Bucket(memoriesBucket)
		chat := memories.Bucket(chatBucketKey(chatID))
		if chat == nil {
			return nil
		}

		return chat.ForEach(func(k, v []byte) error {
			var mem Memory
			if err := json.Unmarshal(v, &mem); err != nil {
				return fmt.Errorf("unmarshal memory: %w", err)
			}
			if strings.ToLower(mem.Fact) == lower {
				found = true
			}
			return nil
		})
	})

	return found, err
}
