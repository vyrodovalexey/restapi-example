package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vyrodovalexey/restapi-example/internal/model"
)

// MemoryStore implements Store interface with in-memory storage.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]model.Item
}

// NewMemoryStore creates a new MemoryStore instance.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]model.Item),
	}
}

// List returns all items from the store.
func (s *MemoryStore) List(ctx context.Context) ([]model.Item, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("list items: %w", ctx.Err())
	default:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]model.Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}

	return items, nil
}

// Get retrieves an item by its ID.
func (s *MemoryStore) Get(ctx context.Context, id string) (*model.Item, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("get item: %w", ctx.Err())
	default:
	}

	if id == "" {
		return nil, ErrInvalidID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	item, exists := s.items[id]
	if !exists {
		return nil, ErrNotFound
	}

	return &item, nil
}

// Create adds a new item to the store and returns the created item with generated ID.
func (s *MemoryStore) Create(ctx context.Context, item *model.Item) (*model.Item, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("create item: %w", ctx.Err())
	default:
	}

	if item == nil {
		return nil, fmt.Errorf("create item: item cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	newItem := model.Item{
		ID:          uuid.New().String(),
		Name:        item.Name,
		Description: item.Description,
		Price:       item.Price,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.items[newItem.ID] = newItem

	return &newItem, nil
}

// Update modifies an existing item in the store.
func (s *MemoryStore) Update(ctx context.Context, id string, item *model.Item) (*model.Item, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("update item: %w", ctx.Err())
	default:
	}

	if id == "" {
		return nil, ErrInvalidID
	}

	if item == nil {
		return nil, fmt.Errorf("update item: item cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.items[id]
	if !exists {
		return nil, ErrNotFound
	}

	updatedItem := model.Item{
		ID:          id,
		Name:        item.Name,
		Description: item.Description,
		Price:       item.Price,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   time.Now().UTC(),
	}

	s.items[id] = updatedItem

	return &updatedItem, nil
}

// Delete removes an item from the store by its ID.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("delete item: %w", ctx.Err())
	default:
	}

	if id == "" {
		return ErrInvalidID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[id]; !exists {
		return ErrNotFound
	}

	delete(s.items, id)

	return nil
}
