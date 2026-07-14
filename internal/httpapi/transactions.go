package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/CTran10/clearance/internal/domain"
	"github.com/CTran10/clearance/internal/funding"
	"github.com/CTran10/clearance/internal/metrics"
	"github.com/CTran10/clearance/internal/transaction"
)

const defaultMaxBodyBytes int64 = 1 << 20

var safeHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Config struct {
	AuthValue         string
	FundingAuthValue  string
	OperatorAuthValue string
	AllowedOrigins    []string
	MaxBodyBytes      int64
	TrustForwardedFor bool
	MetricsEnabled    bool
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type Router struct {
	service        *transaction.Service
	queries        *transaction.QueryService
	fundingService *funding.Service
	limiter        RateLimiter
	config         Config
}

type Option func(*Router)

func WithQueryService(service *transaction.QueryService) Option {
	return func(router *Router) {
		router.queries = service
	}
}

func WithFundingService(service *funding.Service) Option {
	return func(router *Router) {
		router.fundingService = service
	}
}

func NewRouter(service *transaction.Service, limiter RateLimiter, config Config, options ...Option) http.Handler {
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = defaultMaxBodyBytes
	}
	router := &Router{service: service, limiter: limiter, config: config}
	for _, option := range options {
		option(router)
	}
	return router
}

func (r *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if r.config.MetricsEnabled && request.URL.Path == "/metrics" && request.Method == http.MethodGet {
		metrics.Handler().ServeHTTP(response, request)
		return
	}

	recorder := &statusRecorder{ResponseWriter: response}
	defer func() {
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		metrics.Inc("clearance_http_requests_total", metrics.Labels{
			"method": request.Method,
			"path":   metricPath(request),
			"status": strconv.Itoa(status),
		})
	}()

	r.serveHTTP(recorder, request)
}

func (r *Router) serveHTTP(response http.ResponseWriter, request *http.Request) {
	r.setBaseHeaders(response, request)

	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.URL.Path == "/healthz" && request.Method == http.MethodGet {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if request.URL.Path == "/transactions" {
		switch request.Method {
		case http.MethodPost:
			r.createTransaction(response, request)
		case http.MethodGet:
			r.listTransactions(response, request)
		default:
			writeError(response, http.StatusNotFound, "not found")
		}
		return
	}
	if transactionID, ok := transactionIDFromPath(request.URL.Path); ok && request.Method == http.MethodGet {
		r.getTransaction(response, request, transactionID)
		return
	}
	if accountID, ok := depositAccountFromPath(request.URL.Path); ok && request.Method == http.MethodPost {
		r.createDeposit(response, request, accountID)
		return
	}
	writeError(response, http.StatusNotFound, "not found")
}

func (r *Router) createTransaction(response http.ResponseWriter, request *http.Request) {
	if !authorized(request.Header.Get("Authorization"), r.config.AuthValue) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}

	allowed, err := r.limiter.Allow(request.Context(), rateLimitKey(request, r.config.TrustForwardedFor))
	if err != nil {
		writeError(response, http.StatusInternalServerError, "internal error")
		return
	}
	if !allowed {
		writeError(response, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	var payload struct {
		AccountID   string `json:"account_id"`
		MerchantID  string `json:"merchant_id"`
		AmountCents int64  `json:"amount_cents"`
		Currency    string `json:"currency"`
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

func (r *Router) allowRequest(response http.ResponseWriter, request *http.Request) bool {
	allowed, err := r.limiter.Allow(request.Context(), rateLimitKey(request, r.config.TrustForwardedFor))
	if err != nil {
		writeError(response, http.StatusInternalServerError, "internal error")
		return false
	}
	if !allowed {
		writeError(response, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}
	return true
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

func (r *Router) setBaseHeaders(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("X-Frame-Options", "DENY")
	origin := request.Header.Get("Origin")
	for _, allowed := range r.config.AllowedOrigins {
		if origin == allowed {
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Set("Vary", "Origin")
			response.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Correlation-ID")
			response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			return
		}
	}
}

func metricPath(request *http.Request) string {
	switch request.URL.Path {
	case "/healthz", "/transactions":
		return request.URL.Path
	}
	if _, ok := transactionIDFromPath(request.URL.Path); ok {
		return "/transactions/{id}"
	}
	if _, ok := depositAccountFromPath(request.URL.Path); ok {
		return "/accounts/{id}/deposits"
	}
	return "/unknown"
}

func transactionIDFromPath(path string) (string, bool) {
	const prefix = "/transactions/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(path, prefix)
	return id, id != "" && !strings.Contains(id, "/")
}

func depositAccountFromPath(path string) (string, bool) {
	const prefix = "/accounts/"
	const suffix = "/deposits"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	accountID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return accountID, accountID != "" && !strings.Contains(accountID, "/")
}

func authorized(header string, expected string) bool {
	if expected == "" || !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	got := strings.TrimPrefix(header, "Bearer ")
	// timing attacks again (hi, callback to the python dummy-hash thing). a normal got == expected bails on
	// the FIRST wrong byte, so a token starting with the right char takes a hair longer to reject. measure enough
	// requests and you can brute the token one byte at a time. ConstantTimeCompare always checks every byte. == 1 means match
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func rateLimitKey(request *http.Request, trustForwardedFor bool) string {
	if trustForwardedFor {
		if forwarded := firstForwardedFor(request.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return request.RemoteAddr
}

func firstForwardedFor(header string) string {
	for _, part := range strings.Split(header, ",") {
		addr, err := netip.ParseAddr(strings.TrimSpace(part))
		if err == nil {
			return addr.String()
		}
		return ""
	}
	return ""
}

func writeError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"error": message})
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(payload []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(payload)
}
