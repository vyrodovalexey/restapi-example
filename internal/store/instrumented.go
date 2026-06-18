package store

import (
	"context"
	"time"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/observability"
)

// Store operation name constants used as the `operation` metric label. Kept
// here so label cardinality is fixed and documented in one place.
const (
	opList   = "list"
	opGet    = "get"
	opCreate = "create"
	opUpdate = "update"
	opDelete = "delete"
)

// InstrumentedStore decorates a Store with Prometheus instrumentation,
// recording store_operations_total{operation,result} and
// store_operation_duration_seconds{operation} for every operation. It is a
// transparent pass-through wrapper that preserves the Store contract.
type InstrumentedStore struct {
	delegate Store
}

// NewInstrumentedStore wraps the given Store with metrics instrumentation.
func NewInstrumentedStore(delegate Store) *InstrumentedStore {
	return &InstrumentedStore{delegate: delegate}
}

// observe records the duration and success/failure result for an operation.
func observe(operation string, start time.Time, err error) {
	observability.StoreOperationDuration.
		WithLabelValues(operation).
		Observe(time.Since(start).Seconds())

	result := observability.ResultSuccess
	if err != nil {
		result = observability.ResultFailure
	}
	observability.StoreOperationsTotal.
		WithLabelValues(operation, result).
		Inc()
}

// List returns all items, recording instrumentation for the list operation.
func (s *InstrumentedStore) List(ctx context.Context) ([]model.Item, error) {
	start := time.Now()
	items, err := s.delegate.List(ctx)
	observe(opList, start, err)
	return items, err
}

// Get retrieves an item by ID, recording instrumentation for the get operation.
func (s *InstrumentedStore) Get(ctx context.Context, id string) (*model.Item, error) {
	start := time.Now()
	item, err := s.delegate.Get(ctx, id)
	observe(opGet, start, err)
	return item, err
}

// Create adds an item, recording instrumentation for the create operation.
func (s *InstrumentedStore) Create(ctx context.Context, item *model.Item) (*model.Item, error) {
	start := time.Now()
	created, err := s.delegate.Create(ctx, item)
	observe(opCreate, start, err)
	return created, err
}

// Update modifies an item, recording instrumentation for the update operation.
func (s *InstrumentedStore) Update(ctx context.Context, id string, item *model.Item) (*model.Item, error) {
	start := time.Now()
	updated, err := s.delegate.Update(ctx, id, item)
	observe(opUpdate, start, err)
	return updated, err
}

// Delete removes an item, recording instrumentation for the delete operation.
func (s *InstrumentedStore) Delete(ctx context.Context, id string) error {
	start := time.Now()
	err := s.delegate.Delete(ctx, id)
	observe(opDelete, start, err)
	return err
}
