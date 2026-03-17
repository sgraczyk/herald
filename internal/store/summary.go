package store

import bolt "go.etcd.io/bbolt"

// SaveSummary stores or overwrites the conversation summary for a chat.
func (d *DB) SaveSummary(chatID int64, text string) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(summariesBucket)
		return b.Put(chatBucketKey(chatID), []byte(text))
	})
}

// ClearSummary deletes the conversation summary for a chat.
func (d *DB) ClearSummary(chatID int64) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(summariesBucket)
		return b.Delete(chatBucketKey(chatID))
	})
}

// GetSummary returns the conversation summary for a chat, or empty string if none.
func (d *DB) GetSummary(chatID int64) (string, error) {
	var summary string
	err := d.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(summariesBucket)
		v := b.Get(chatBucketKey(chatID))
		if v != nil {
			summary = string(v)
		}
		return nil
	})
	return summary, err
}
