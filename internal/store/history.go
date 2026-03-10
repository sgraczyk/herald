package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/sgraczyk/herald/internal/provider"
	bolt "go.etcd.io/bbolt"
)

// EstimateTokens returns a rough token count for text using the len/4
// heuristic. Non-empty text always returns at least 1.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	n := len(text) / 4
	if n == 0 {
		return 1
	}
	return n
}

// Append adds a message to the chat's history and prunes old messages
// if the count exceeds limit.
func (d *DB) Append(chatID int64, msg provider.Message, limit int) error {
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
		if limit > 0 {
			return prune(chat, limit)
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

// ListWithTokenBudget returns messages for a chat, trimming the oldest
// messages so the total estimated tokens fit within budget. If budget is
// zero or negative, all messages are returned (token trimming disabled).
// The message count hard cap (limit) is applied first when limit > 0.
// A single message that exceeds the budget is still returned to avoid
// returning an empty history.
func (d *DB) ListWithTokenBudget(chatID int64, limit, budget int) ([]provider.Message, error) {
	msgs, err := d.List(chatID)
	if err != nil {
		return nil, err
	}

	// Apply message count hard cap.
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}

	// Skip token trimming if disabled or nothing to trim.
	if budget <= 0 || len(msgs) <= 1 {
		return msgs, nil
	}

	// Sum tokens from newest to oldest. Keep as many recent messages as fit.
	total := 0
	cutoff := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		total += EstimateTokens(msgs[i].Content)
		if total > budget {
			cutoff = i + 1
			break
		}
	}

	// Never return empty: keep at least the most recent message.
	if cutoff >= len(msgs) {
		cutoff = len(msgs) - 1
	}

	trimmed := msgs[cutoff:]
	removed := len(msgs) - len(trimmed)
	if removed > 0 {
		slog.Info("history token trim",
			slog.Int64("chat_id", chatID),
			slog.Int("messages_removed", removed),
			slog.Int("tokens_used", total-tokensForMessages(msgs[:cutoff])),
			slog.Int("token_budget", budget),
		)
	}

	return trimmed, nil
}

// tokensForMessages sums estimated tokens for a slice of messages.
func tokensForMessages(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	return total
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
