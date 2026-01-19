package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vyrodovalexey/restapi-example/internal/model"
)

func TestNewMemoryStore(t *testing.T) {
	// Act
	store := NewMemoryStore()

	// Assert
	if store == nil {
		t.Fatal("NewMemoryStore() returned nil")
	}
	if store.items == nil {
		t.Error("items map should be initialized")
	}
}

func TestMemoryStore_Create(t *testing.T) {
	tests := []struct {
		name    string
		item    *model.Item
		wantErr bool
	}{
		{
			name: "valid item",
			item: &model.Item{
				Name:        "Test Item",
				Description: "A test item",
				Price:       9.99,
			},
			wantErr: false,
		},
		{
			name: "item with zero price",
			item: &model.Item{
				Name:  "Free Item",
				Price: 0,
			},
			wantErr: false,
		},
		{
			name: "item with empty description",
			item: &model.Item{
				Name:  "Simple Item",
				Price: 5.00,
			},
			wantErr: false,
		},
		{
			name:    "nil item",
			item:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			store := NewMemoryStore()
			ctx := context.Background()

			// Act
			created, err := store.Create(ctx, tt.item)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Errorf("Create() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}

			if created == nil {
				t.Fatal("Create() returned nil item")
			}

			if created.ID == "" {
				t.Error("Create() should generate an ID")
			}
			if created.Name != tt.item.Name {
				t.Errorf("Name = %s, want %s", created.Name, tt.item.Name)
			}
			if created.Description != tt.item.Description {
				t.Errorf("Description = %s, want %s", created.Description, tt.item.Description)
			}
			if created.Price != tt.item.Price {
				t.Errorf("Price = %f, want %f", created.Price, tt.item.Price)
			}
			if created.CreatedAt.IsZero() {
				t.Error("CreatedAt should be set")
			}
			if created.UpdatedAt.IsZero() {
				t.Error("UpdatedAt should be set")
			}
		})
	}
}

func TestMemoryStore_Create_ContextCancellation(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	item := &model.Item{
		Name:  "Test Item",
		Price: 9.99,
	}

	// Act
	created, err := store.Create(ctx, item)

	// Assert
	if err == nil {
		t.Error("Create() expected error for cancelled context")
	}
	if created != nil {
		t.Error("Create() should return nil for cancelled context")
	}
}

func TestMemoryStore_Get(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()

	item := &model.Item{
		Name:        "Test Item",
		Description: "A test item",
		Price:       9.99,
	}
	created, _ := store.Create(ctx, item)

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing item",
			id:      created.ID,
			wantErr: nil,
		},
		{
			name:    "non-existing item",
			id:      "non-existent-id",
			wantErr: ErrNotFound,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := store.Get(ctx, tt.id)

			// Assert
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Get() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Get() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}

			if got == nil {
				t.Fatal("Get() returned nil item")
			}

			if got.ID != created.ID {
				t.Errorf("ID = %s, want %s", got.ID, created.ID)
			}
			if got.Name != created.Name {
				t.Errorf("Name = %s, want %s", got.Name, created.Name)
			}
		})
	}
}

func TestMemoryStore_Get_ContextCancellation(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act
	got, err := store.Get(ctx, "some-id")

	// Assert
	if err == nil {
		t.Error("Get() expected error for cancelled context")
	}
	if got != nil {
		t.Error("Get() should return nil for cancelled context")
	}
}

func TestMemoryStore_List(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*MemoryStore, context.Context)
		wantCount int
	}{
		{
			name:      "empty store",
			setup:     func(_ *MemoryStore, _ context.Context) {},
			wantCount: 0,
		},
		{
			name: "single item",
			setup: func(s *MemoryStore, ctx context.Context) {
				_, _ = s.Create(ctx, &model.Item{Name: "Item 1", Price: 10})
			},
			wantCount: 1,
		},
		{
			name: "multiple items",
			setup: func(s *MemoryStore, ctx context.Context) {
				_, _ = s.Create(ctx, &model.Item{Name: "Item 1", Price: 10})
				_, _ = s.Create(ctx, &model.Item{Name: "Item 2", Price: 20})
				_, _ = s.Create(ctx, &model.Item{Name: "Item 3", Price: 30})
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			store := NewMemoryStore()
			ctx := context.Background()
			tt.setup(store, ctx)

			// Act
			items, err := store.List(ctx)

			// Assert
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}

			if len(items) != tt.wantCount {
				t.Errorf("List() returned %d items, want %d", len(items), tt.wantCount)
			}
		})
	}
}

func TestMemoryStore_List_ContextCancellation(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act
	items, err := store.List(ctx)

	// Assert
	if err == nil {
		t.Error("List() expected error for cancelled context")
	}
	if items != nil {
		t.Error("List() should return nil for cancelled context")
	}
}

func TestMemoryStore_Update(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()

	original := &model.Item{
		Name:        "Original Item",
		Description: "Original description",
		Price:       9.99,
	}
	created, _ := store.Create(ctx, original)

	tests := []struct {
		name    string
		id      string
		update  *model.Item
		wantErr error
	}{
		{
			name: "valid update",
			id:   created.ID,
			update: &model.Item{
				Name:        "Updated Item",
				Description: "Updated description",
				Price:       19.99,
			},
			wantErr: nil,
		},
		{
			name: "non-existing item",
			id:   "non-existent-id",
			update: &model.Item{
				Name:  "Updated Item",
				Price: 19.99,
			},
			wantErr: ErrNotFound,
		},
		{
			name: "empty id",
			id:   "",
			update: &model.Item{
				Name:  "Updated Item",
				Price: 19.99,
			},
			wantErr: ErrInvalidID,
		},
		{
			name:    "nil item",
			id:      created.ID,
			update:  nil,
			wantErr: nil, // Will be an error but not ErrNotFound or ErrInvalidID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate store for each test to avoid state pollution
			store := NewMemoryStore()
			ctx := context.Background()
			created, _ := store.Create(ctx, original)

			id := tt.id
			if tt.name == "valid update" || tt.name == "nil item" {
				id = created.ID
			}

			// Act
			updated, err := store.Update(ctx, id, tt.update)

			// Assert
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Update() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Update() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if tt.update == nil {
				if err == nil {
					t.Error("Update() expected error for nil item")
				}
				return
			}

			if err != nil {
				t.Fatalf("Update() unexpected error: %v", err)
			}

			if updated == nil {
				t.Fatal("Update() returned nil item")
			}

			if updated.ID != id {
				t.Errorf("ID = %s, want %s", updated.ID, id)
			}
			if updated.Name != tt.update.Name {
				t.Errorf("Name = %s, want %s", updated.Name, tt.update.Name)
			}
			if updated.Description != tt.update.Description {
				t.Errorf("Description = %s, want %s", updated.Description, tt.update.Description)
			}
			if updated.Price != tt.update.Price {
				t.Errorf("Price = %f, want %f", updated.Price, tt.update.Price)
			}
			if updated.CreatedAt != created.CreatedAt {
				t.Error("CreatedAt should not change on update")
			}
			if !updated.UpdatedAt.After(created.UpdatedAt) && updated.UpdatedAt != created.UpdatedAt {
				t.Error("UpdatedAt should be updated")
			}
		})
	}
}

func TestMemoryStore_Update_ContextCancellation(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	update := &model.Item{
		Name:  "Updated Item",
		Price: 19.99,
	}

	// Act
	updated, err := store.Update(ctx, "some-id", update)

	// Assert
	if err == nil {
		t.Error("Update() expected error for cancelled context")
	}
	if updated != nil {
		t.Error("Update() should return nil for cancelled context")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()

	item := &model.Item{
		Name:  "Test Item",
		Price: 9.99,
	}
	created, _ := store.Create(ctx, item)

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing item",
			id:      created.ID,
			wantErr: nil,
		},
		{
			name:    "non-existing item",
			id:      "non-existent-id",
			wantErr: ErrNotFound,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: ErrInvalidID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate store for each test
			store := NewMemoryStore()
			ctx := context.Background()
			created, _ := store.Create(ctx, item)

			id := tt.id
			if tt.name == "existing item" {
				id = created.ID
			}

			// Act
			err := store.Delete(ctx, id)

			// Assert
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Delete() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Delete() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Delete() unexpected error: %v", err)
			}

			// Verify item is deleted
			_, err = store.Get(ctx, id)
			if err != ErrNotFound {
				t.Error("Item should be deleted")
			}
		})
	}
}

func TestMemoryStore_Delete_ContextCancellation(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act
	err := store.Delete(ctx, "some-id")

	// Assert
	if err == nil {
		t.Error("Delete() expected error for cancelled context")
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()
	numGoroutines := 100
	numOperations := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Act - Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Create
				item := &model.Item{
					Name:  "Test Item",
					Price: float64(id * j),
				}
				created, err := store.Create(ctx, item)
				if err != nil {
					return
				}

				// Get
				_, _ = store.Get(ctx, created.ID)

				// List
				_, _ = store.List(ctx)

				// Update
				update := &model.Item{
					Name:  "Updated Item",
					Price: float64(id * j * 2),
				}
				_, _ = store.Update(ctx, created.ID, update)

				// Delete
				_ = store.Delete(ctx, created.ID)
			}
		}(i)
	}

	wg.Wait()

	// Assert - No panic occurred and store is in consistent state
	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() after concurrent access failed: %v", err)
	}

	// All items should be deleted
	if len(items) != 0 {
		t.Logf("Store has %d items remaining after concurrent operations", len(items))
	}
}

func TestMemoryStore_ConcurrentReads(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()

	// Create some items first
	for i := 0; i < 10; i++ {
		_, _ = store.Create(ctx, &model.Item{
			Name:  "Test Item",
			Price: float64(i),
		})
	}

	numGoroutines := 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Act - Run concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = store.List(ctx)
			}
		}()
	}

	wg.Wait()

	// Assert - No panic occurred
	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() after concurrent reads failed: %v", err)
	}
	if len(items) != 10 {
		t.Errorf("Expected 10 items, got %d", len(items))
	}
}

func TestMemoryStore_ConcurrentWrites(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()
	numGoroutines := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Act - Run concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, _ = store.Create(ctx, &model.Item{
				Name:  "Test Item",
				Price: float64(id),
			})
		}(i)
	}

	wg.Wait()

	// Assert
	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() after concurrent writes failed: %v", err)
	}
	if len(items) != numGoroutines {
		t.Errorf("Expected %d items, got %d", numGoroutines, len(items))
	}
}

func TestMemoryStore_UniqueIDs(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()
	numItems := 100
	ids := make(map[string]bool)

	// Act
	for i := 0; i < numItems; i++ {
		created, err := store.Create(ctx, &model.Item{
			Name:  "Test Item",
			Price: float64(i),
		})
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		if ids[created.ID] {
			t.Errorf("Duplicate ID generated: %s", created.ID)
		}
		ids[created.ID] = true
	}

	// Assert
	if len(ids) != numItems {
		t.Errorf("Expected %d unique IDs, got %d", numItems, len(ids))
	}
}

func TestMemoryStore_Timestamps(t *testing.T) {
	// Arrange
	store := NewMemoryStore()
	ctx := context.Background()

	before := time.Now().UTC()

	// Act - Create
	item := &model.Item{
		Name:  "Test Item",
		Price: 9.99,
	}
	created, err := store.Create(ctx, item)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	after := time.Now().UTC()

	// Assert - CreatedAt and UpdatedAt should be set
	if created.CreatedAt.Before(before) || created.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, should be between %v and %v", created.CreatedAt, before, after)
	}
	if created.UpdatedAt.Before(before) || created.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, should be between %v and %v", created.UpdatedAt, before, after)
	}
	if created.CreatedAt != created.UpdatedAt {
		t.Error("CreatedAt and UpdatedAt should be equal on creation")
	}

	// Wait a bit and update
	time.Sleep(10 * time.Millisecond)
	beforeUpdate := time.Now().UTC()

	update := &model.Item{
		Name:  "Updated Item",
		Price: 19.99,
	}
	updated, err := store.Update(ctx, created.ID, update)
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	afterUpdate := time.Now().UTC()

	// Assert - CreatedAt should not change, UpdatedAt should be updated
	if updated.CreatedAt != created.CreatedAt {
		t.Error("CreatedAt should not change on update")
	}
	if updated.UpdatedAt.Before(beforeUpdate) || updated.UpdatedAt.After(afterUpdate) {
		t.Errorf("UpdatedAt = %v, should be between %v and %v", updated.UpdatedAt, beforeUpdate, afterUpdate)
	}
}

func TestMemoryStore_ImplementsInterface(t *testing.T) {
	// Assert that MemoryStore implements Store interface
	var _ Store = (*MemoryStore)(nil)
}
