package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/transaction"
)

var safeHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func (r *Router) createTransaction(response http.ResponseWriter, request *http.Request) {
	if !authorized(request.Header.Get("Authorization"), r.config.AuthValue) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}

	if !r.allowRequest(response, request) {
		return
	}

	var payload struct {
		AccountID string `json:"account_id"`
		MerchantID string `json:"merchant_id"`
		AmountCents int64  `json:"amount_cents"`
		Currency string `json:"currency"`
	}
	request.Body = http.MaxBytesReader(response, request.Body, r.config.MaxBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid request")
		return
	}

	correlationID := request.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = domain.NewID("trace")
	}
	if !safeHeaderPattern.MatchString(correlationID) {
		writeError(response, http.StatusBadRequest, "invalid request")
		return
	}

	result, err := r.service.Create(
		request.Context(),
		transaction.CreateRequest{
			AccountID:   payload.AccountID,
			MerchantID:  payload.MerchantID,
			AmountCents: payload.AmountCents,
			Currency:    payload.Currency,
		},
		transaction.RequestMetadata{
			IdempotencyKey: request.Header.Get("Idempotency-Key"),
			CorrelationID:  correlationID,
		},
	)
	if err != nil {
		r.writeServiceError(response, err)
		return
	}

	writeJSON(response, http.StatusAccepted, map[string]string{
		"transaction_id": result.TransactionID,
		"status":         string(result.Status),
		"correlation_id": result.CorrelationID,
	})
}

func (r *Router) getTransaction(response http.ResponseWriter, request *http.Request, transactionID string) {
	if r.queries == nil {
		writeError(response, http.StatusNotFound, "not found")
		return
	}
	authorization := request.Header.Get("Authorization")
	if !authorized(authorization, r.config.AuthValue) && !authorized(authorization, r.config.OperatorAuthValue) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !r.allowRequest(response, request) {
		return
	}
	detail, err := r.queries.Get(request.Context(), transactionID)
	if err != nil {
		switch {
		case errors.Is(err, transaction.ErrInvalidQuery):
			writeError(response, http.StatusBadRequest, "invalid request")
		case errors.Is(err, transaction.ErrNotFound):
			writeError(response, http.StatusNotFound, "transaction not found")
		default:
			writeError(response, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(response, http.StatusOK, detail)
}

func (r *Router) listTransactions(response http.ResponseWriter, request *http.Request) {
	if r.queries == nil {
		writeError(response, http.StatusNotFound, "not found")
		return
	}
	if !authorized(request.Header.Get("Authorization"), r.config.OperatorAuthValue) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !r.allowRequest(response, request) {
		return
	}
	limit := 0
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(response, http.StatusBadRequest, "invalid request")
			return
		}
		limit = parsed
	}
	page, err := r.queries.List(request.Context(), transaction.ListFilter{
		AccountID: request.URL.Query().Get("account_id"),
		Status:    domain.TransactionStatus(request.URL.Query().Get("status")),
		Kind:      domain.TransactionKind(request.URL.Query().Get("kind")),
		Limit:     limit,
		Cursor:    request.URL.Query().Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, transaction.ErrInvalidQuery) {
			writeError(response, http.StatusBadRequest, "invalid request")
		} else {
			writeError(response, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(response, http.StatusOK, page)
}

func (r *Router) writeServiceError(response http.ResponseWriter, err error) {
	// one place to turn internal errors into http status codes. errors.Is "unwraps" the chain to find a sentinel
	// even if it got wrapped 3 layers deep with %w — that's why i wrapped instead of stringifying earlier.
	// the default case is the safety net: anything i didn't explicitly map becomes a generic 500, never a leak
	switch {
	case errors.Is(err, transaction.ErrInvalidRequest):
		writeError(response, http.StatusBadRequest, "invalid request")
	case errors.Is(err, transaction.ErrIdempotencyConflict):
		writeError(response, http.StatusConflict, "idempotency key conflict")
	default:
		writeError(response, http.StatusInternalServerError, "internal error")
	}
}
