// Package store keeps the products.
package store

import "example.com/shop/internal/db"

// Store is a product store.
type Store struct {
	conn *db.Conn
}

// Open connects to dsn.
func Open(dsn string) *Store {
	return &Store{conn: db.Dial(dsn)}
}

// Count returns the number of products.
func (s *Store) Count() int {
	return s.conn.Rows("products") + cacheHits
}
