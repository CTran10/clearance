package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/CTran10/clearance/internal/funding"
	"github.com/CTran10/clearance/internal/transaction"
)

const defaultMaxBodyBytes int64 = 1 << 20

type Config struct {
	QueryService *transaction.QueryService
	FundingService *funding.Service
	AuthValue string
	FundingAuthValue string
	OperatorAuthValue string
	AllowedOrigins []string
	MaxBodyBytes int64
	TrustForwardedFor bool
	MetricsEnabled bool
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type Router struct {
	service *transaction.Service
	limiter RateLimiter
	config Config
}

func NewRouter(service *transaction.Service, limiter RateLimiter, config Config) http.Handler {
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = defaultMaxBodyBytes
	}
	return &Router{service: service, limiter: limiter, config: config}
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
	first, _, _ := strings.Cut(header, ",")
	addr, err := netip.ParseAddr(strings.TrimSpace(first))
	if err != nil {
		return ""
	}
	return addr.String()
}

func writeError(response http.ResponseWriter, status int, message string) {
	writeJSON(response, status, map[string]string{"error": message})
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
