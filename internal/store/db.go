// Package store provides persistent storage for conversation history and
// user memories using bbolt.
package store

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var (
	messagesBucket  = []byte("messages")
	memoriesBucket  = []byte("memories")
)

// DB wraps a bbolt database for Herald storage.
type DB struct {
	bolt *bolt.DB
}

// Open opens (or creates) the bbolt database at path.
func Open(path string) (*DB, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Ensure top-level buckets exist.
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(messagesBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(memoriesBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}

	return &DB{bolt: db}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.bolt.Close()
}
