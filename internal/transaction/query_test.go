package transaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CTran10/clearance/internal/domain"
)

func TestQueryServiceGetsTransactionAndPaginatesDeterministically(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := &queryMemoryStore{items: []Detail{
		{ID: "txn_3", Kind: domain.TransactionPayment, AccountID: "acct_123", Status: domain.TransactionAuthorized, CreatedAt: now},
		{ID: "txn_2", Kind: domain.TransactionPayment, AccountID: "acct_123", Status: domain.TransactionPending, CreatedAt: now.Add(-time.Minute)},
		{ID: "txn_1", Kind: domain.TransactionPayment, AccountID: "acct_123", Status: domain.TransactionFailed, CreatedAt: now.Add(-2 * time.Minute)},
	}}
	service := NewQueryService(store)

	detail, err := service.Get(context.Background(), "txn_2")
	if err != nil || detail.ID != "txn_2" {
		t.Fatalf("Get = %#v, %v", detail, err)
	}
	page, err := service.List(context.Background(), ListFilter{AccountID: "acct_123", Limit: 2})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %#v", page)
	}
	next, err := service.List(context.Background(), ListFilter{AccountID: "acct_123", Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("second List returned error: %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != "txn_1" || next.NextCursor != "" {
		t.Fatalf("second page = %#v", next)
	}
}

func TestQueryServiceValidatesFiltersAndMissingRows(t *testing.T) {
	t.Parallel()

	service := NewQueryService(&queryMemoryStore{})
	if _, err := service.Get(context.Background(), "../unsafe"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unsafe id error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.Get(context.Background(), "txn_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id error = %v, want ErrNotFound", err)
	}
	if _, err := service.List(context.Background(), ListFilter{Limit: 10}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("missing account error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.List(context.Background(), ListFilter{AccountID: "acct_123", Limit: 101}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("large limit error = %v, want ErrInvalidQuery", err)
	}
	if _, err := service.List(context.Background(), ListFilter{AccountID: "acct_123", Cursor: "not-base64"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("bad cursor error = %v, want ErrInvalidQuery", err)
	}
}

type queryMemoryStore struct {
	items []Detail
}

func (s *queryMemoryStore) GetTransaction(_ context.Context, id string) (Detail, bool, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return Detail{}, false, nil
}

func (s *queryMemoryStore) ListTransactions(_ context.Context, filter StoreListFilter) ([]Detail, error) {
	items := make([]Detail, 0, filter.Limit+1)
	for _, item := range s.items {
		if item.AccountID != filter.AccountID {
			continue
		}
		if !filter.BeforeCreatedAt.IsZero() && (item.CreatedAt.After(filter.BeforeCreatedAt) ||
			(item.CreatedAt.Equal(filter.BeforeCreatedAt) && item.ID >= filter.BeforeID)) {
			continue
		}
		items = append(items, item)
		if len(items) == filter.Limit+1 {
			break
		}
	}
	return items, nil
}
