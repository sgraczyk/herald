package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/sgraczyk/herald/internal/provider"
	bolt "go.etcd.io/bbolt"
)

// Append adds a message to the chat's history and prunes old messages
// if the count exceeds the configured history limit.
func (d *DB) Append(chatID int64, msg provider.Message) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		messages := tx.Bucket(messagesBucket)
		chatKey := chatBucketKey(chatID)

		chat, err := messages.CreateBucketIfNotExists(chatKey)
		if err != nil {
			return fmt.Errorf("create chat bucket: %w", err)
		}

		seq, err := chat.NextSequence()
		if err != nil {
			return fmt.Errorf("next sequence: %w", err)
		}

		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal message: %w", err)
		}

		if err := chat.Put(uint64Key(seq), data); err != nil {
			return fmt.Errorf("put message: %w", err)
		}

		// Prune oldest messages if over limit.
		if d.historyLimit > 0 {
			return prune(chat, d.historyLimit)
		}
		return nil
	})
}

// List returns all messages for a chat, ordered by sequence.
func (d *DB) List(chatID int64) ([]provider.Message, error) {
	var msgs []provider.Message

	err := d.bolt.View(func(tx *bolt.Tx) error {
		messages := tx.Bucket(messagesBucket)
		chat := messages.Bucket(chatBucketKey(chatID))
		if chat == nil {
			return nil
		}

		return chat.ForEach(func(k, v []byte) error {
			var msg provider.Message
			if err := json.Unmarshal(v, &msg); err != nil {
				return fmt.Errorf("unmarshal message: %w", err)
			}
			msgs = append(msgs, msg)
			return nil
		})
	})

	return msgs, err
}

// Clear deletes all messages for a chat.
func (d *DB) Clear(chatID int64) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		messages := tx.Bucket(messagesBucket)
		key := chatBucketKey(chatID)

		// Delete and recreate the bucket to reset sequence.
		if messages.Bucket(key) != nil {
			if err := messages.DeleteBucket(key); err != nil {
				return fmt.Errorf("delete chat bucket: %w", err)
			}
		}
		return nil
	})
}

// Count returns the number of messages for a chat.
func (d *DB) Count(chatID int64) (int, error) {
	var count int

	err := d.bolt.View(func(tx *bolt.Tx) error {
		messages := tx.Bucket(messagesBucket)
		chat := messages.Bucket(chatBucketKey(chatID))
		if chat == nil {
			return nil
		}
		count = chat.Stats().KeyN
		return nil
	})

	return count, err
}

func prune(bucket *bolt.Bucket, limit int) error {
	// Count keys via cursor (Stats may be stale within a transaction).
	count := 0
	c := bucket.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		count++
	}

	if count <= limit {
		return nil
	}

	// Collect keys to delete (can't delete during iteration).
	toDelete := count - limit
	keys := make([][]byte, 0, toDelete)
	for k, _ := c.First(); k != nil && len(keys) < toDelete; k, _ = c.Next() {
		cp := make([]byte, len(k))
		copy(cp, k)
		keys = append(keys, cp)
	}

	for _, k := range keys {
		if err := bucket.Delete(k); err != nil {
			return fmt.Errorf("delete old message: %w", err)
		}
	}
	return nil
}

func chatBucketKey(chatID int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(chatID))
	return b
}

func uint64Key(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
