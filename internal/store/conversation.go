package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sgraczyk/herald/internal/provider"
	bolt "go.etcd.io/bbolt"
)

var archivesBucket = []byte("archives")

// ArchivedConversation holds a snapshot of an archived conversation.
type ArchivedConversation struct {
	Timestamp time.Time          `json:"timestamp"`
	Messages  []provider.Message `json:"messages"`
	Summary   string             `json:"summary,omitempty"`
}

// ArchiveConversation snapshots the current history and summary into the
// archives bucket, then clears both. Memory is untouched. Returns false
// if there was nothing to archive.
func (d *DB) ArchiveConversation(chatID int64) (bool, error) {
	var archived bool

	err := d.bolt.Update(func(tx *bolt.Tx) error {
		chatKey := chatBucketKey(chatID)

		// Read messages.
		messages := tx.Bucket(messagesBucket)
		chat := messages.Bucket(chatKey)
		if chat == nil {
			return nil
		}

		var msgs []provider.Message
		if err := chat.ForEach(func(k, v []byte) error {
			var m provider.Message
			if err := json.Unmarshal(v, &m); err != nil {
				return fmt.Errorf("unmarshal message: %w", err)
			}
			msgs = append(msgs, m)
			return nil
		}); err != nil {
			return err
		}

		if len(msgs) == 0 {
			return nil
		}

		// Read summary.
		summaries := tx.Bucket(summariesBucket)
		var summary string
		if v := summaries.Get(chatKey); v != nil {
			summary = string(v)
		}

		// Build archive entry.
		entry := ArchivedConversation{
			Timestamp: time.Now(),
			Messages:  msgs,
			Summary:   summary,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal archive: %w", err)
		}

		// Store in archives bucket.
		archives, err := tx.CreateBucketIfNotExists(archivesBucket)
		if err != nil {
			return fmt.Errorf("create archives bucket: %w", err)
		}
		chatArchives, err := archives.CreateBucketIfNotExists(chatKey)
		if err != nil {
			return fmt.Errorf("create chat archives bucket: %w", err)
		}
		tsKey := uint64Key(uint64(entry.Timestamp.UnixNano()))
		if err := chatArchives.Put(tsKey, data); err != nil {
			return fmt.Errorf("put archive: %w", err)
		}

		// Clear messages.
		if err := messages.DeleteBucket(chatKey); err != nil {
			return fmt.Errorf("delete message bucket: %w", err)
		}

		// Clear summary.
		if err := summaries.Delete(chatKey); err != nil {
			return fmt.Errorf("delete summary: %w", err)
		}

		archived = true
		return nil
	})

	return archived, err
}

// PruneArchived removes the oldest archived conversations for a chat so that
// the total count does not exceed limit. Each deletion uses the same
// transaction for efficiency but operates on individual keys for safety.
func (d *DB) PruneArchived(chatID int64, limit int) error {
	if limit <= 0 {
		return nil
	}

	return d.bolt.Update(func(tx *bolt.Tx) error {
		archives := tx.Bucket(archivesBucket)
		if archives == nil {
			return nil
		}
		chatArchives := archives.Bucket(chatBucketKey(chatID))
		if chatArchives == nil {
			return nil
		}

		// Collect all keys in one pass.
		var all [][]byte
		c := chatArchives.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			cp := make([]byte, len(k))
			copy(cp, k)
			all = append(all, cp)
		}

		toDelete := len(all) - limit
		if toDelete <= 0 {
			return nil
		}

		keys := all[:toDelete]

		for _, k := range keys {
			if err := chatArchives.Delete(k); err != nil {
				return fmt.Errorf("delete old archive: %w", err)
			}
		}
		return nil
	})
}

// ClearArchived removes all archived conversations for a chat.
func (d *DB) ClearArchived(chatID int64) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		archives := tx.Bucket(archivesBucket)
		if archives == nil {
			return nil
		}
		key := chatBucketKey(chatID)
		if archives.Bucket(key) == nil {
			return nil
		}
		return archives.DeleteBucket(key)
	})
}

// ListArchived returns all archived conversations for a chat, ordered by time.
func (d *DB) ListArchived(chatID int64) ([]ArchivedConversation, error) {
	var convs []ArchivedConversation

	err := d.bolt.View(func(tx *bolt.Tx) error {
		archives := tx.Bucket(archivesBucket)
		if archives == nil {
			return nil
		}
		chatArchives := archives.Bucket(chatBucketKey(chatID))
		if chatArchives == nil {
			return nil
		}

		return chatArchives.ForEach(func(k, v []byte) error {
			var conv ArchivedConversation
			if err := json.Unmarshal(v, &conv); err != nil {
				return fmt.Errorf("unmarshal archive: %w", err)
			}
			convs = append(convs, conv)
			return nil
		})
	})

	return convs, err
}
