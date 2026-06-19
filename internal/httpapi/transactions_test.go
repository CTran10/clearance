package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/transaction"
)

var errTestInternal = errors.New("database password leaked if exposed")

func testAuthValue() string {
	return "local test bearer value"
}

func TestTransactionHandlerRequiresBearerToken(t *testing.T) {
	t.Parallel()

	handler := NewRouter(
		transaction.NewService(transaction.NewMemoryStore()),
		NewMemoryRateLimiter(10),
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

	store := transaction.NewMemoryStore()
	handler := NewRouter(
		transaction.NewService(store),
		NewMemoryRateLimiter(10),
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
		NewMemoryRateLimiter(10),
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

	store := transaction.NewMemoryStore()
	handler := NewRouter(
		transaction.NewService(store),
		NewMemoryRateLimiter(0),
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

type failingStore struct{}

func (failingStore) FindIdempotency(context.Context, string) (transaction.IdempotencyRecord, bool, error) {
	return transaction.IdempotencyRecord{}, false, nil
}

func (failingStore) Create(context.Context, transaction.IdempotencyRecord, domain.OutboxEvent) error {
	return errTestInternal
}
