package store

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/observability"
)

// fakeStore is a configurable Store double used to verify the
// InstrumentedStore decorator delegates correctly and records both the success
// and failure metric paths.
type fakeStore struct {
	listFn   func(ctx context.Context) ([]model.Item, error)
	getFn    func(ctx context.Context, id string) (*model.Item, error)
	createFn func(ctx context.Context, item *model.Item) (*model.Item, error)
	updateFn func(ctx context.Context, id string, item *model.Item) (*model.Item, error)
	deleteFn func(ctx context.Context, id string) error

	listCalls   int
	getCalls    int
	createCalls int
	updateCalls int
	deleteCalls int
}

func (f *fakeStore) List(ctx context.Context) ([]model.Item, error) {
	f.listCalls++
	return f.listFn(ctx)
}

func (f *fakeStore) Get(ctx context.Context, id string) (*model.Item, error) {
	f.getCalls++
	return f.getFn(ctx, id)
}

func (f *fakeStore) Create(ctx context.Context, item *model.Item) (*model.Item, error) {
	f.createCalls++
	return f.createFn(ctx, item)
}

func (f *fakeStore) Update(ctx context.Context, id string, item *model.Item) (*model.Item, error) {
	f.updateCalls++
	return f.updateFn(ctx, id, item)
}

func (f *fakeStore) Delete(ctx context.Context, id string) error {
	f.deleteCalls++
	return f.deleteFn(ctx, id)
}

func storeOpCount(t *testing.T, operation, result string) float64 {
	t.Helper()
	return testutil.ToFloat64(
		observability.StoreOperationsTotal.WithLabelValues(operation, result),
	)
}

func TestNewInstrumentedStore(t *testing.T) {
	delegate := NewMemoryStore()
	is := NewInstrumentedStore(delegate)
	if is == nil {
		t.Fatal("NewInstrumentedStore returned nil")
	}
	if is.delegate != delegate {
		t.Error("delegate not stored")
	}
}

// TestInstrumentedStore_Delegates verifies every method forwards to the
// underlying store and returns its results unchanged, recording success
// metrics.
func TestInstrumentedStore_Delegates(t *testing.T) {
	ctx := context.Background()
	wantItem := &model.Item{ID: "1", Name: "widget", Price: 1.5}
	wantList := []model.Item{*wantItem}

	fake := &fakeStore{
		listFn:   func(context.Context) ([]model.Item, error) { return wantList, nil },
		getFn:    func(context.Context, string) (*model.Item, error) { return wantItem, nil },
		createFn: func(context.Context, *model.Item) (*model.Item, error) { return wantItem, nil },
		updateFn: func(context.Context, string, *model.Item) (*model.Item, error) { return wantItem, nil },
		deleteFn: func(context.Context, string) error { return nil },
	}
	is := NewInstrumentedStore(fake)

	// List
	gotList, err := is.List(ctx)
	if err != nil || len(gotList) != 1 {
		t.Fatalf("List() = %v, %v; want 1 item, nil", gotList, err)
	}
	if fake.listCalls != 1 {
		t.Errorf("List delegate calls = %d, want 1", fake.listCalls)
	}

	// Get
	gotItem, err := is.Get(ctx, "1")
	if err != nil || gotItem != wantItem {
		t.Fatalf("Get() = %v, %v; want item, nil", gotItem, err)
	}
	if fake.getCalls != 1 {
		t.Errorf("Get delegate calls = %d, want 1", fake.getCalls)
	}

	// Create
	gotItem, err = is.Create(ctx, wantItem)
	if err != nil || gotItem != wantItem {
		t.Fatalf("Create() = %v, %v; want item, nil", gotItem, err)
	}
	if fake.createCalls != 1 {
		t.Errorf("Create delegate calls = %d, want 1", fake.createCalls)
	}

	// Update
	gotItem, err = is.Update(ctx, "1", wantItem)
	if err != nil || gotItem != wantItem {
		t.Fatalf("Update() = %v, %v; want item, nil", gotItem, err)
	}
	if fake.updateCalls != 1 {
		t.Errorf("Update delegate calls = %d, want 1", fake.updateCalls)
	}

	// Delete
	if err := is.Delete(ctx, "1"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if fake.deleteCalls != 1 {
		t.Errorf("Delete delegate calls = %d, want 1", fake.deleteCalls)
	}
}

// TestInstrumentedStore_RecordsSuccessAndFailureMetrics verifies the metric
// result label reflects whether the delegate returned an error, for every
// operation. It is table-driven over the five operations and both outcomes.
func TestInstrumentedStore_RecordsSuccessAndFailureMetrics(t *testing.T) {
	opErr := errors.New("boom")

	tests := []struct {
		name      string
		operation string
		wantErr   bool
		invoke    func(is *InstrumentedStore) error
	}{
		{
			name:      "list success",
			operation: opList,
			invoke: func(is *InstrumentedStore) error {
				_, err := is.List(context.Background())
				return err
			},
		},
		{
			name:      "get failure",
			operation: opGet,
			wantErr:   true,
			invoke: func(is *InstrumentedStore) error {
				_, err := is.Get(context.Background(), "x")
				return err
			},
		},
		{
			name:      "create success",
			operation: opCreate,
			invoke: func(is *InstrumentedStore) error {
				_, err := is.Create(context.Background(), &model.Item{})
				return err
			},
		},
		{
			name:      "update failure",
			operation: opUpdate,
			wantErr:   true,
			invoke: func(is *InstrumentedStore) error {
				_, err := is.Update(context.Background(), "x", &model.Item{})
				return err
			},
		},
		{
			name:      "delete success",
			operation: opDelete,
			invoke: func(is *InstrumentedStore) error {
				return is.Delete(context.Background(), "x")
			},
		},
		{
			name:      "delete failure",
			operation: opDelete,
			wantErr:   true,
			invoke: func(is *InstrumentedStore) error {
				return is.Delete(context.Background(), "x")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := observability.ResultSuccess
			var retErr error
			if tt.wantErr {
				result = observability.ResultFailure
				retErr = opErr
			}

			fake := &fakeStore{
				listFn:   func(context.Context) ([]model.Item, error) { return nil, retErr },
				getFn:    func(context.Context, string) (*model.Item, error) { return nil, retErr },
				createFn: func(context.Context, *model.Item) (*model.Item, error) { return nil, retErr },
				updateFn: func(context.Context, string, *model.Item) (*model.Item, error) { return nil, retErr },
				deleteFn: func(context.Context, string) error { return retErr },
			}
			is := NewInstrumentedStore(fake)

			before := storeOpCount(t, tt.operation, result)

			err := tt.invoke(is)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			after := storeOpCount(t, tt.operation, result)
			if after-before != 1 {
				t.Errorf("store_operations_total{%s,%s} delta = %v, want 1",
					tt.operation, result, after-before)
			}
		})
	}
}

// TestInstrumentedStore_ObservesDuration verifies the duration histogram
// records a sample per operation.
func TestInstrumentedStore_ObservesDuration(t *testing.T) {
	observability.StoreOperationDuration.Reset()

	fake := &fakeStore{
		listFn: func(context.Context) ([]model.Item, error) { return nil, nil },
	}
	is := NewInstrumentedStore(fake)

	if _, err := is.List(context.Background()); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if count := testutil.CollectAndCount(
		observability.StoreOperationDuration,
	); count == 0 {
		t.Error("store_operation_duration_seconds recorded no series")
	}
}

// TestInstrumentedStore_ImplementsStore is a compile-time + runtime assertion
// that the decorator satisfies the Store interface.
func TestInstrumentedStore_ImplementsStore(t *testing.T) {
	var _ Store = (*InstrumentedStore)(nil)

	is := NewInstrumentedStore(NewMemoryStore())
	var s Store = is
	if s == nil {
		t.Fatal("InstrumentedStore does not satisfy Store")
	}
}
