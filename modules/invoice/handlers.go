package invoice

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/arandu-io/framework/security"
)

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
		return
	}

	customerID := r.URL.Query().Get("customer")
	if customerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "customer is required"})
		return
	}

	invoices, err := m.svc.ByCustomer(r.Context(), actor, customerID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

func (m *Module) outstanding(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
		return
	}

	total, err := m.svc.Outstanding(r.Context(), actor)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"outstanding_cents": total,
		"outstanding":       float64(total) / centsInUnit,
	})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, security.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		panic(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
