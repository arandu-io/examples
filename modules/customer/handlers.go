package customer

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
)

// Handlers are thin on purpose: extract the input, delegate to the service,
// render the answer. No business rule and no repository access lives here --
// `aru doctor` complains when a handler imports the data package for anything
// beyond the query type.
//
// They answer JSON. The view layer arrives with porang in phase 2, and when it
// does these same handlers gain a branch that returns an HTML fragment: the
// module contract does not change.

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		unauthorized(w)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	q := data.Query{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
		Sort:   r.URL.Query().Get("sort"),
	}

	customers, err := m.svc.List(r.Context(), actor, q)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, customers)
}

func (m *Module) show(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		unauthorized(w)
		return
	}

	found, err := m.svc.Get(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

// showDocument returns the unmasked registration number, which needs its own
// permission. It is the route that demonstrates why "can read the record" and
// "can read every field" are different questions.
func (m *Module) showDocument(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		unauthorized(w)
		return
	}

	document, err := m.svc.FullDocument(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"document": document})
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		unauthorized(w)
		return
	}

	in := CreateRequest{
		Name:     r.PostFormValue("name"),
		Email:    r.PostFormValue("email"),
		Document: r.PostFormValue("document"),
	}

	created, err := m.svc.Create(r.Context(), actor, in)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (m *Module) destroy(w http.ResponseWriter, r *http.Request) {
	actor, err := m.subject(r)
	if err != nil {
		unauthorized(w)
		return
	}

	if err := m.svc.Delete(r.Context(), actor, r.PathValue("id")); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail turns a domain error into a status code, in one place.
//
// Note what it does NOT do: it never writes err.Error() into the response for an
// authorization failure. The reason a policy said no is information about the
// system, and it belongs in the log, which is where it goes.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var invalid validation.Errors
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": invalid})

	case errors.Is(err, security.ErrForbidden):
		observability.Log(r.Context()).Warn("authorization denied", "error", err)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})

	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})

	case errors.Is(err, ErrEmailTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})

	case errors.Is(err, ErrSortNotList):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

	default:
		// Anything unrecognized is a bug, and a bug in development belongs on the
		// debug page with its queries and dumps -- not swallowed into a 500.
		panic(err)
	}
}

func unauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
