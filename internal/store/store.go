// Package store provides data storage interfaces and implementations.
package store

import (
	"context"
	"errors"

	"github.com/vyrodovalexey/restapi-example/internal/model"
)

// Store errors.
var (
	ErrNotFound      = errors.New("item not found")
	ErrAlreadyExists = errors.New("item already exists")
	ErrInvalidID     = errors.New("invalid item ID")
	ErrNilItem       = errors.New("item cannot be nil")
)

// Store defines the interface for item storage operations.
type Store interface {
	// List returns all items from the store.
	List(ctx context.Context) ([]model.Item, error)

	// Get retrieves an item by its ID.
	Get(ctx context.Context, id string) (*model.Item, error)

	// Create adds a new item to the store and returns the created item with generated ID.
	Create(ctx context.Context, item *model.Item) (*model.Item, error)

	// Update modifies an existing item in the store.
	Update(ctx context.Context, id string, item *model.Item) (*model.Item, error)

	// Delete removes an item from the store by its ID.
	Delete(ctx context.Context, id string) error
}
