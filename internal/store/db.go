package store

import (
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

var messagesBucket = []byte("messages")

// DB wraps a bbolt database for Herald storage.
type DB struct {
	bolt         *bolt.DB
	historyLimit int
}

// Open opens (or creates) the bbolt database at path.
// historyLimit sets the maximum number of messages per chat (0 = no limit).
func Open(path string, historyLimit int) (*DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Ensure top-level buckets exist.
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(messagesBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}

	return &DB{bolt: db, historyLimit: historyLimit}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.bolt.Close()
}
