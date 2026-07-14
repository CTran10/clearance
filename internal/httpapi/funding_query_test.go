package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/funding"
	"github.com/CTran10/clearance/internal/transaction"
)

func TestFundingEndpointUsesSeparateCredentialAndReturnsCreated(t *testing.T) {
	t.Parallel()

	fundingStore := newHTTPFundingStore()
	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), FundingAuthValue: "funding-secret"},
		WithFundingService(funding.NewService(fundingStore, funding.Config{MaxAmountCents: 1_000_000})),
	)
	body := []byte(`{"amount_cents":25000,"currency":"USD","funding_source":"demo-operator","external_reference":"transfer-123","operator_reason":"seed demo account"}`)

	unauthorized := httptest.NewRequest(http.MethodPost, "/accounts/acct_123/deposits", bytes.NewReader(body))
	unauthorized.Header.Set("Authorization", "Bearer "+testAuthValue())
	unauthorized.Header.Set("Idempotency-Key", "fund-123")
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("transaction credential status = %d, want 401", unauthorizedResponse.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/accounts/acct_123/deposits", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer funding-secret")
	request.Header.Set("Idempotency-Key", "fund-123")
	request.Header.Set("X-Correlation-ID", "trace-123")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 body=%s", response.Code, response.Body.String())
	}
}

func TestTransactionReadEndpointsEnforceCredentialScopes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	queryStore := &httpQueryStore{items: []transaction.Detail{{
		ID: "txn_123", Kind: domain.TransactionPayment, AccountID: "acct_123", MerchantID: "merchant_123",
		AmountCents: 12_550, Currency: "USD", Status: domain.TransactionAuthorized,
		RiskLevel: domain.RiskLow, CorrelationID: "trace-123", CreatedAt: now, UpdatedAt: now,
	}}}
	handler := NewRouter(
		transactionService(newTransactionMemoryStore()),
		newMemoryRateLimiter(10),
		Config{AuthValue: testAuthValue(), OperatorAuthValue: "operator-secret"},
		WithQueryService(transaction.NewQueryService(queryStore)),
	)

	getRequest := httptest.NewRequest(http.MethodGet, "/transactions/txn_123", nil)
	getRequest.Header.Set("Authorization", "Bearer "+testAuthValue())
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var detail transaction.Detail
	if err := json.NewDecoder(getResponse.Body).Decode(&detail); err != nil || detail.ID != "txn_123" {
		t.Fatalf("get payload = %#v, %v", detail, err)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/transactions?account_id=acct_123&limit=25", nil)
	listRequest.Header.Set("Authorization", "Bearer "+testAuthValue())
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusUnauthorized {
		t.Fatalf("transaction credential list status = %d, want 401", listResponse.Code)
	}

	listRequest = httptest.NewRequest(http.MethodGet, "/transactions?account_id=acct_123&limit=25", nil)
	listRequest.Header.Set("Authorization", "Bearer operator-secret")
	listResponse = httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("operator list status = %d body=%s", listResponse.Code, listResponse.Body.String())
	}
}

type httpFundingStore struct {
	records map[string]funding.IdempotencyRecord
}

func newHTTPFundingStore() *httpFundingStore {
	return &httpFundingStore{records: make(map[string]funding.IdempotencyRecord)}
}

func (s *httpFundingStore) FindDepositIdempotency(_ context.Context, key string) (funding.IdempotencyRecord, bool, error) {
	record, ok := s.records[key]
	return record, ok, nil
}

func (s *httpFundingStore) CreateDeposit(_ context.Context, deposit funding.Deposit, _ domain.OutboxEvent) (funding.DepositResponse, error) {
	response := funding.DepositResponse{
		DepositID: deposit.Transaction.ID, TransactionID: deposit.Transaction.ID,
		Status: deposit.Transaction.Status, AccountID: deposit.Transaction.AccountID,
		AmountCents: deposit.Transaction.AmountCents, Currency: deposit.Transaction.Currency,
		BalanceAfterCents: deposit.Transaction.AmountCents, CorrelationID: deposit.Transaction.CorrelationID,
		CreatedAt: deposit.Transaction.CreatedAt,
	}
	s.records[deposit.IdempotencyKey] = funding.IdempotencyRecord{RequestHash: deposit.RequestHash, Response: response}
	return response, nil
}

type httpQueryStore struct {
	items []transaction.Detail
}

func (s *httpQueryStore) GetTransaction(_ context.Context, id string) (transaction.Detail, bool, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return transaction.Detail{}, false, nil
}

func (s *httpQueryStore) ListTransactions(_ context.Context, _ transaction.StoreListFilter) ([]transaction.Detail, error) {
	return append([]transaction.Detail(nil), s.items...), nil
}
