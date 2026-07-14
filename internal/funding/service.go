package funding

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
	"unicode"

	"github.com/CTran10/clearance/internal/domain"
)

var (
	ErrInvalidRequest            = errors.New("invalid deposit request")
	ErrIdempotencyConflict       = errors.New("deposit idempotency key reused with different payload")
	ErrExternalReferenceConflict = errors.New("deposit external reference already used")
)

var (
	safeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
)

const settlementAccount = "external-settlement"

type Config struct {
	MaxAmountCents int64
}

type DepositRequest struct {
	AccountID         string
	AmountCents       int64
	Currency          string
	FundingSource     string
	ExternalReference string
}

type RequestMetadata struct {
	IdempotencyKey string
	CorrelationID  string
	OperatorReason string
}

type DepositResponse struct {
	DepositID         string                   `json:"deposit_id"`
	TransactionID     string                   `json:"transaction_id"`
	Status            domain.TransactionStatus `json:"status"`
	AccountID         string                   `json:"account_id"`
	AmountCents       int64                    `json:"amount_cents"`
	Currency          string                   `json:"currency"`
	BalanceAfterCents int64                    `json:"balance_after_cents"`
	CorrelationID     string                   `json:"correlation_id"`
	CreatedAt         time.Time                `json:"created_at"`
}

type IdempotencyRecord struct {
	RequestHash string
	Response    DepositResponse
}

type Deposit struct {
	IdempotencyKey string
	RequestHash    string
	OperatorReason string
	Transaction    domain.Transaction
}

type Store interface {
	FindDepositIdempotency(ctx context.Context, key string) (IdempotencyRecord, bool, error)
	CreateDeposit(ctx context.Context, deposit Deposit, event domain.OutboxEvent) (DepositResponse, error)
}

type Service struct {
	store          Store
	maxAmountCents int64
}

func NewService(store Store, config Config) *Service {
	maxAmount := config.MaxAmountCents
	if maxAmount <= 0 {
		maxAmount = 100_000_000
	}
	return &Service{store: store, maxAmountCents: maxAmount}
}

func (s *Service) Deposit(ctx context.Context, request DepositRequest, metadata RequestMetadata) (DepositResponse, error) {
	request, metadata, err := s.validate(request, metadata)
	if err != nil {
		return DepositResponse{}, err
	}

	requestHash := hashRequest(request)
	existing, ok, err := s.store.FindDepositIdempotency(ctx, metadata.IdempotencyKey)
	if err != nil {
		return DepositResponse{}, fmt.Errorf("find deposit idempotency key: %w", err)
	}
	if ok {
		if existing.RequestHash != requestHash {
			return DepositResponse{}, ErrIdempotencyConflict
		}
		return existing.Response, nil
	}

	now := time.Now().UTC()
	transaction := domain.Transaction{
		ID:            domain.NewID("dep"),
		Kind:          domain.TransactionDeposit,
		AccountID:     request.AccountID,
		FundingSource: request.FundingSource,
		ExternalRef:   request.ExternalReference,
		AmountCents:   request.AmountCents,
		Currency:      request.Currency,
		Status:        domain.TransactionAuthorized,
		CorrelationID: metadata.CorrelationID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	payload, err := json.Marshal(domain.FundsDeposited{
		DepositID: transaction.ID, AccountID: transaction.AccountID,
		AmountCents: transaction.AmountCents, Currency: transaction.Currency,
		FundingSource: transaction.FundingSource, ExternalReference: transaction.ExternalRef,
		CorrelationID: transaction.CorrelationID,
	})
	if err != nil {
		return DepositResponse{}, fmt.Errorf("marshal funds deposited event: %w", err)
	}
	event := domain.NewOutboxEvent(
		domain.EventFundsDeposited,
		transaction.ID,
		transaction.AccountID,
		transaction.CorrelationID,
		payload,
	)
	deposit := Deposit{
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		OperatorReason: metadata.OperatorReason,
		Transaction:    transaction,
	}
	response, err := s.store.CreateDeposit(ctx, deposit, event)
	if err == nil {
		return response, nil
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		existing, ok, findErr := s.store.FindDepositIdempotency(ctx, metadata.IdempotencyKey)
		if findErr != nil {
			return DepositResponse{}, fmt.Errorf("find deposit idempotency key after conflict: %w", findErr)
		}
		if ok && existing.RequestHash == requestHash {
			return existing.Response, nil
		}
	}
	return DepositResponse{}, fmt.Errorf("create deposit: %w", err)
}

func (s *Service) validate(request DepositRequest, metadata RequestMetadata) (DepositRequest, RequestMetadata, error) {
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	request.FundingSource = strings.TrimSpace(request.FundingSource)
	request.ExternalReference = strings.TrimSpace(request.ExternalReference)
	metadata.IdempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)
	metadata.CorrelationID = strings.TrimSpace(metadata.CorrelationID)
	metadata.OperatorReason = strings.TrimSpace(metadata.OperatorReason)
	if metadata.CorrelationID == "" {
		metadata.CorrelationID = domain.NewID("trace")
	}

	if !safeTokenPattern.MatchString(request.AccountID) || request.AccountID == "clearing" || request.AccountID == settlementAccount ||
		!safeTokenPattern.MatchString(request.FundingSource) || !safeTokenPattern.MatchString(request.ExternalReference) ||
		!safeTokenPattern.MatchString(metadata.IdempotencyKey) || !safeTokenPattern.MatchString(metadata.CorrelationID) ||
		!currencyPattern.MatchString(request.Currency) || request.AmountCents <= 0 || request.AmountCents > s.maxAmountCents ||
		!validReason(metadata.OperatorReason) {
		return DepositRequest{}, RequestMetadata{}, ErrInvalidRequest
	}
	return request, metadata, nil
}

func validReason(reason string) bool {
	if len(reason) == 0 || len(reason) > 256 {
		return false
	}
	for _, value := range reason {
		if unicode.IsControl(value) {
			return false
		}
	}
	return true
}

func hashRequest(request DepositRequest) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"deposit|%s|%d|%s|%s|%s",
		request.AccountID,
		request.AmountCents,
		request.Currency,
		request.FundingSource,
		request.ExternalReference,
	)))
	return hex.EncodeToString(sum[:])
}
