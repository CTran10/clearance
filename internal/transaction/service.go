package transaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/CTran10/clearance/internal/domain"
)

var (
	ErrInvalidRequest      = errors.New("invalid transaction request")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
)

var (
	safeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
)

type CreateRequest struct {
	AccountID   string
	MerchantID  string
	AmountCents int64
	Currency    string
}

type RequestMetadata struct {
	IdempotencyKey string
	CorrelationID  string
}

type CreateResponse struct {
	TransactionID string
	Status        domain.TransactionStatus
	CorrelationID string
}

type IdempotencyRecord struct {
	Key          string
	RequestHash  string
	Transaction  domain.Transaction
	CreateResult CreateResponse
}

type Store interface {
	FindIdempotency(ctx context.Context, key string) (IdempotencyRecord, bool, error)
	Create(ctx context.Context, record IdempotencyRecord, event domain.OutboxEvent) error
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, request CreateRequest, metadata RequestMetadata) (CreateResponse, error) {
	normalized, err := validate(request, metadata)
	if err != nil {
		return CreateResponse{}, err
	}

	requestHash := hashRequest(normalized)
	existing, ok, err := s.store.FindIdempotency(ctx, metadata.IdempotencyKey)
	if err != nil {
		return CreateResponse{}, fmt.Errorf("find idempotency key: %w", err)
	}
	if ok {
		if existing.RequestHash != requestHash {
			return CreateResponse{}, ErrIdempotencyConflict
		}
		return existing.CreateResult, nil
	}

	transaction := domain.Transaction{
		ID:            domain.NewID("txn"),
		Kind:          domain.TransactionPayment,
		AccountID:     normalized.AccountID,
		MerchantID:    normalized.MerchantID,
		AmountCents:   normalized.AmountCents,
		Currency:      normalized.Currency,
		Status:        domain.TransactionPending,
		CorrelationID: metadata.CorrelationID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	response := CreateResponse{
		TransactionID: transaction.ID,
		Status:        transaction.Status,
		CorrelationID: transaction.CorrelationID,
	}
	payload, err := json.Marshal(transaction)
	if err != nil {
		return CreateResponse{}, fmt.Errorf("marshal transaction created event: %w", err)
	}
	record := IdempotencyRecord{
		Key:          metadata.IdempotencyKey,
		RequestHash:  requestHash,
		Transaction:  transaction,
		CreateResult: response,
	}
	event := domain.NewOutboxEvent(
		domain.EventTransactionCreated,
		transaction.ID,
		transaction.AccountID,
		metadata.CorrelationID,
		payload,
	)
	if err := s.store.Create(ctx, record, event); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			existing, ok, findErr := s.store.FindIdempotency(ctx, metadata.IdempotencyKey)
			if findErr != nil {
				return CreateResponse{}, fmt.Errorf("find idempotency key after conflict: %w", findErr)
			}
			if ok && existing.RequestHash == requestHash {
				return existing.CreateResult, nil
			}
		}
		return CreateResponse{}, fmt.Errorf("create transaction: %w", err)
	}
	return response, nil
}

func validate(request CreateRequest, metadata RequestMetadata) (CreateRequest, error) {
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.MerchantID = strings.TrimSpace(request.MerchantID)
	metadata.IdempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)

	if !safeTokenPattern.MatchString(metadata.IdempotencyKey) ||
		!safeTokenPattern.MatchString(request.AccountID) ||
		!safeTokenPattern.MatchString(request.MerchantID) ||
		!currencyPattern.MatchString(request.Currency) ||
		request.AmountCents <= 0 {
		return CreateRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func hashRequest(request CreateRequest) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%d|%s",
		request.AccountID,
		request.MerchantID,
		request.AmountCents,
		request.Currency,
	)))
	return hex.EncodeToString(sum[:])
}
