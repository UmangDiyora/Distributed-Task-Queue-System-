package postgres

import (
	"strings"
)

// Close closes the database connection
func (p *PostgresStore) Close() error {
	return p.db.Close()
}

// Helper functions for error handling

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "UNIQUE constraint")
}

func isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "foreign key") ||
		strings.Contains(err.Error(), "violates foreign key constraint")
}
