package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/app"
	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

const maxBodyBytes = 1 << 20

type Server struct {
	service *app.Service
	log     *slog.Logger
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func New(service *app.Service, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Server{service: service, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/accounts", s.createAccount)
	mux.HandleFunc("GET /v1/accounts/{id}", s.getAccount)
	mux.HandleFunc("POST /v1/transfers", s.createTransfer)
	mux.HandleFunc("GET /v1/journal", s.listJournal)
	mux.HandleFunc("GET /v1/audit", s.listAudit)
	mux.HandleFunc("GET /v1/risk-events", s.listRiskEvents)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		rw.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		rw.Header().Set("Cache-Control", "no-store")
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Referrer-Policy", "no-referrer")
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic recovered", "error", recovered)
				if rw.status == 0 {
					writeError(rw, http.StatusInternalServerError, "internal_error", "internal server error")
				}
			}
			s.log.Info("request",
				"method", r.Method, "path", r.URL.Path,
				"status", rw.status,
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}()
		next.ServeHTTP(rw, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	var cmd app.CreateAccountCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeDomainError(w, err)
		return
	}
	account, err := s.service.CreateAccount(r.Context(), actor, cmd)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	account, err := s.service.GetAccount(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	var cmd app.TransferCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeDomainError(w, err)
		return
	}
	cmd.IdempotencyKey = r.Header.Get("Idempotency-Key")
	result, err := s.service.Transfer(r.Context(), actor, cmd)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
		w.Header().Set("Idempotent-Replayed", "true")
	}
	writeJSON(w, status, result)
}

func (s *Server) listJournal(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	limit, err := queryLimit(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	items, err := s.service.ListJournal(r.Context(), actor, limit)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	limit, err := queryLimit(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	items, err := s.service.ListAudit(r.Context(), actor, limit)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listRiskEvents(w http.ResponseWriter, r *http.Request) {
	actor, err := principalFromRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	limit, err := queryLimit(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	items, err := s.service.ListRiskEvents(r.Context(), actor, limit)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func principalFromRequest(r *http.Request) (domain.Principal, error) {
	p := domain.Principal{
		ID:   strings.TrimSpace(r.Header.Get("X-Principal-ID")),
		Role: domain.Role(strings.TrimSpace(r.Header.Get("X-Principal-Role"))),
	}
	if !p.Valid() {
		return domain.Principal{}, domain.ErrForbidden
	}
	return p, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domain.ErrUnsupportedMedia
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return domain.ErrRequestTooLarge
		}
		return fmt.Errorf("%w: malformed JSON: %v", domain.ErrInvalidInput, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request must contain one JSON object", domain.ErrInvalidInput)
	}
	return nil
}

func queryLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 100, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("%w: limit must be an integer from 1 to 1000", domain.ErrInvalidInput)
	}
	return limit, nil
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "request is not authorised")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrInsufficientFunds):
		writeError(w, http.StatusConflict, "insufficient_funds", "insufficient funds")
	case errors.Is(err, domain.ErrCurrencyMismatch):
		writeError(w, http.StatusConflict, "currency_mismatch", "account currencies differ")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key was used for a different request")
	case errors.Is(err, domain.ErrTransferLimit):
		writeError(w, http.StatusUnprocessableEntity, "transfer_limit", "transfer exceeds configured limit")
	case errors.Is(err, domain.ErrRequestTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB")
	case errors.Is(err, domain.ErrUnsupportedMedia):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}
