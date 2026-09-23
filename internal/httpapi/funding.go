package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CTran10/clearance/internal/funding"
)

func (r *Router) createDeposit(response http.ResponseWriter, request *http.Request, accountID string) {
	if r.fundingService == nil {
		writeError(response, http.StatusNotFound, "not found")
		return
	}
	if !authorized(request.Header.Get("Authorization"), r.config.FundingAuthValue) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !r.allowRequest(response, request) {
		return
	}
	var payload struct {
		AmountCents       int64  `json:"amount_cents"`
		Currency          string `json:"currency"`
		FundingSource     string `json:"funding_source"`
		ExternalReference string `json:"external_reference"`
		OperatorReason    string `json:"operator_reason"`
	}
	request.Body = http.MaxBytesReader(response, request.Body, r.config.MaxBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := r.fundingService.Deposit(
		request.Context(),
		funding.DepositRequest{
			AccountID: accountID, AmountCents: payload.AmountCents, Currency: payload.Currency,
			FundingSource: payload.FundingSource, ExternalReference: payload.ExternalReference,
		},
		funding.RequestMetadata{
			IdempotencyKey: request.Header.Get("Idempotency-Key"),
			CorrelationID:  request.Header.Get("X-Correlation-ID"),
			OperatorReason: payload.OperatorReason,
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, funding.ErrInvalidRequest):
			writeError(response, http.StatusBadRequest, "invalid request")
		case errors.Is(err, funding.ErrIdempotencyConflict), errors.Is(err, funding.ErrExternalReferenceConflict):
			writeError(response, http.StatusConflict, "deposit conflict")
		default:
			writeError(response, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(response, http.StatusCreated, result)
}
