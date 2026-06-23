package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/transaction"
)

var errTestInternal = errors.New("database password leaked if exposed")

func testAuthValue() string {
	return "local test bearer value"
}

type memoryRateLimiter struct {
	mu        sync.Mutex
	remaining int
}

func newMemoryRateLimiter(allowed int) *memoryRateLimiter {
	return &memoryRateLimiter{remaining: allowed}
}

func (l *memoryRateLimiter) Allow(context.Context, string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.remaining <= 0 {
		return false, nil
	}
	l.remaining--
	return true, nil
}

func TestTransactionHandlerRequiresBearerToken(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue()},
	)
	request := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(`{}`))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestTransactionHandlerCreatesPendingTransaction(t *testing.T) {
	t.Parallel()

	store := newTransactionMemoryStore()
	handler := NewRouter(
		transactionService(store),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), AllowedOrigins: []string{"https://console.example"}},
	)
	body := bytes.NewBufferString(`{
		"account_id":"acct_123",
		"merchant_id":"merchant_123",
		"amount_cents":12550,
		"currency":"usd"
	}`)
	request := httptest.NewRequest(http.MethodPost, "/transactions", body)
	request.Header.Set("Authorization", "Bearer "+testAuthValue())
	request.Header.Set("Idempotency-Key", "idem-123")
	request.Header.Set("X-Correlation-ID", "trace-123")
	request.Header.Set("Origin", "https://console.example")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusAccepted, response.Body.String())
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example" {
		t.Fatalf("cors origin = %q, want configured origin", got)
	}

	var payload struct {
		TransactionID string                   `json:"transaction_id"`
		Status        domain.TransactionStatus `json:"status"`
		CorrelationID string                   `json:"correlation_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TransactionID == "" {
		t.Fatal("transaction_id should be populated")
	}
	if payload.Status != domain.TransactionPending {
		t.Fatalf("status = %q, want %q", payload.Status, domain.TransactionPending)
	}
	if payload.CorrelationID != "trace-123" {
		t.Fatalf("correlation_id = %q, want trace-123", payload.CorrelationID)
	}
	if len(store.OutboxEvents()) != 1 {
		t.Fatal("create should write one outbox event")
	}
}

func TestTransactionHandlerHidesInternalErrors(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transaction.NewService(failingStore{}),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue()},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/transactions",
		bytes.NewBufferString(`{"account_id":"acct_123","merchant_id":"merchant_123","amount_cents":100,"currency":"USD"}`),
	)
	request.Header.Set("Authorization", "Bearer "+testAuthValue())
	request.Header.Set("Idempotency-Key", "idem-123")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	// the failingStore i wired up errors with a fake "database password" string in its message ON PURPOSE.
	// the actual assertion is: that string must NOT appear in the response body. classic leak — you catch an
	// error and lazily do w.Write([]byte(err.Error())), and now your stack traces / db creds get shipped to the
	// client. user gets a boring "internal error", the juicy details go to the logs only. testing the absence of a thing!
	if bytes.Contains(response.Body.Bytes(), []byte("database password")) {
		t.Fatal("response leaked internal error details")
	}
}

func TestTransactionHandlerRateLimitsBeforeCreatingTransaction(t *testing.T) {
	t.Parallel()

	store := newTransactionMemoryStore()
	handler := NewRouter(
		transactionService(store),
		newMemoryRateLimiter(0),
		Config{AuthValue: testAuthValue()},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/transactions",
		bytes.NewBufferString(`{"account_id":"acct_123","merchant_id":"merchant_123","amount_cents":100,"currency":"USD"}`),
	)
	request.Header.Set("Authorization", "Bearer "+testAuthValue())
	request.Header.Set("Idempotency-Key", "idem-123")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if len(store.OutboxEvents()) != 0 {
		t.Fatal("rate-limited request should not create a transaction")
	}
}

func TestTransactionHandlerRateLimitKeyStripsRemotePort(t *testing.T) {
	t.Parallel()

	limiter := &recordingLimiter{allowed: true}
	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		limiter,
		Config{AuthValue: testAuthValue()},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/transactions",
		bytes.NewBufferString(`{"account_id":"acct_123","merchant_id":"merchant_123","amount_cents":100,"currency":"USD"}`),
	)
	request.RemoteAddr = "203.0.113.10:49152"
	request.Header.Set("Authorization", "Bearer "+testAuthValue())
	request.Header.Set("Idempotency-Key", "idem-123")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if limiter.key != "203.0.113.10" {
		t.Fatalf("rate limit key = %q, want host without ephemeral port", limiter.key)
	}
}

func TestTransactionHandlerRateLimitKeyCanUseTrustedForwardedFor(t *testing.T) {
	t.Parallel()

	limiter := &recordingLimiter{allowed: true}
	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		limiter,
		Config{AuthValue: testAuthValue(), TrustForwardedFor: true},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/transactions",
		bytes.NewBufferString(`{"account_id":"acct_123","merchant_id":"merchant_123","amount_cents":100,"currency":"USD"}`),
	)
	request.RemoteAddr = "10.0.0.5:12345"
	request.Header.Set("Authorization", "Bearer "+testAuthValue())
	request.Header.Set("Idempotency-Key", "idem-123")
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.5")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if limiter.key != "198.51.100.7" {
		t.Fatalf("rate limit key = %q, want validated forwarded client ip", limiter.key)
	}
}

type failingStore struct{}

func (failingStore) FindIdempotency(context.Context, string) (transaction.IdempotencyRecord, bool, error) {
	return transaction.IdempotencyRecord{}, false, nil
}

func (failingStore) Create(context.Context, transaction.IdempotencyRecord, domain.OutboxEvent) error {
	return errTestInternal
}

func transactionService(store transaction.Store) *transaction.Service {
	return transaction.NewService(store)
}

type transactionMemoryStore struct {
	mu         sync.Mutex
	idempotent map[string]transaction.IdempotencyRecord
	outbox     []domain.OutboxEvent
}

func newTransactionMemoryStore() *transactionMemoryStore {
	return &transactionMemoryStore{idempotent: make(map[string]transaction.IdempotencyRecord)}
}

func (s *transactionMemoryStore) FindIdempotency(_ context.Context, key string) (transaction.IdempotencyRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.idempotent[key]
	return record, ok, nil
}

func (s *transactionMemoryStore) Create(_ context.Context, record transaction.IdempotencyRecord, event domain.OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.idempotent[record.Key]; ok {
		if existing.RequestHash != record.RequestHash {
			return transaction.ErrIdempotencyConflict
		}
		return nil
	}
	s.idempotent[record.Key] = record
	s.outbox = append(s.outbox, event)
	return nil
}

func (s *transactionMemoryStore) OutboxEvents() []domain.OutboxEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]domain.OutboxEvent(nil), s.outbox...)
}

type recordingLimiter struct {
	key     string
	allowed bool
}

func (l *recordingLimiter) Allow(_ context.Context, key string) (bool, error) {
	l.key = key
	return l.allowed, nil
}
