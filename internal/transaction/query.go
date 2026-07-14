package transaction

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/domain"
)

var (
	ErrInvalidQuery = errors.New("invalid transaction query")
	ErrNotFound     = errors.New("transaction not found")
)

type Detail struct {
	ID                string                   `json:"transaction_id"`
	Kind              domain.TransactionKind   `json:"kind"`
	AccountID         string                   `json:"account_id"`
	MerchantID        string                   `json:"merchant_id,omitempty"`
	FundingSource     string                   `json:"funding_source,omitempty"`
	ExternalReference string                   `json:"external_reference,omitempty"`
	AmountCents       int64                    `json:"amount_cents"`
	Currency          string                   `json:"currency"`
	Status            domain.TransactionStatus `json:"status"`
	RiskLevel         domain.RiskLevel         `json:"risk_level,omitempty"`
	RiskReason        string                   `json:"risk_reason,omitempty"`
	CorrelationID     string                   `json:"correlation_id"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type ListFilter struct {
	AccountID string
	Status    domain.TransactionStatus
	Kind      domain.TransactionKind
	Limit     int
	Cursor    string
}

type StoreListFilter struct {
	AccountID       string
	Status          domain.TransactionStatus
	Kind            domain.TransactionKind
	Limit           int
	BeforeCreatedAt time.Time
	BeforeID        string
}

type Page struct {
	Items      []Detail `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type QueryStore interface {
	GetTransaction(ctx context.Context, id string) (Detail, bool, error)
	ListTransactions(ctx context.Context, filter StoreListFilter) ([]Detail, error)
}

type QueryService struct {
	store QueryStore
}

func NewQueryService(store QueryStore) *QueryService {
	return &QueryService{store: store}
}

func (s *QueryService) Get(ctx context.Context, id string) (Detail, error) {
	if !safeTokenPattern.MatchString(id) {
		return Detail{}, ErrInvalidQuery
	}
	detail, ok, err := s.store.GetTransaction(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("get transaction: %w", err)
	}
	if !ok {
		return Detail{}, ErrNotFound
	}
	return detail, nil
}

func (s *QueryService) List(ctx context.Context, filter ListFilter) (Page, error) {
	if filter.Limit == 0 {
		filter.Limit = 25
	}
	if !safeTokenPattern.MatchString(filter.AccountID) || filter.Limit < 1 || filter.Limit > 100 ||
		!validStatus(filter.Status) || !validKind(filter.Kind) {
		return Page{}, ErrInvalidQuery
	}
	storeFilter := StoreListFilter{
		AccountID: filter.AccountID,
		Status:    filter.Status,
		Kind:      filter.Kind,
		Limit:     filter.Limit,
	}
	if filter.Cursor != "" {
		cursor, err := decodeCursor(filter.Cursor)
		if err != nil {
			return Page{}, ErrInvalidQuery
		}
		storeFilter.BeforeCreatedAt = cursor.CreatedAt
		storeFilter.BeforeID = cursor.ID
	}
	items, err := s.store.ListTransactions(ctx, storeFilter)
	if err != nil {
		return Page{}, fmt.Errorf("list transactions: %w", err)
	}
	page := Page{Items: items}
	if len(page.Items) > filter.Limit {
		page.Items = page.Items[:filter.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(cursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return page, nil
}

func validStatus(status domain.TransactionStatus) bool {
	switch status {
	case "", domain.TransactionPending, domain.TransactionAuthorized, domain.TransactionFailed:
		return true
	default:
		return false
	}
}

func validKind(kind domain.TransactionKind) bool {
	switch kind {
	case "", domain.TransactionPayment, domain.TransactionDeposit:
		return true
	default:
		return false
	}
}

type cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeCursor(value cursor) string {
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(value string) (cursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor{}, err
	}
	var decoded cursor
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return cursor{}, err
	}
	if decoded.CreatedAt.IsZero() || !safeTokenPattern.MatchString(decoded.ID) {
		return cursor{}, ErrInvalidQuery
	}
	return decoded, nil
}
