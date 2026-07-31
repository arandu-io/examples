package customer

import (
	"encoding/json"
	"log/slog"
	"time"
)

// Customer is the entity. It has no persistence methods: this is not Active
// Record, and a type that can save itself is a type that can save itself from
// anywhere.
//
// Document holds a national registration number. It is the field that makes this
// example worth reading: it is exactly the kind of value that must not reach a
// log, a dump or an error page, and the two methods below are what stop it.
type Customer struct {
	ID        string
	TenantID  string
	Name      string
	Email     string
	Document  string
	CreatedAt time.Time
}

// MarshalJSON keeps the document out of any response, log or dump. Without it, a
// single observability.Dump(ctx, "customer", c) would publish it on the debug
// page -- which is the most common way a value like this leaks.
func (c Customer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}{ID: c.ID, Name: c.Name, Email: c.Email})
}

// LogValue implements slog.LogValuer, so passing the whole customer to a log call
// records the identifiers and nothing else. This is the safe default: a careless
// log line cannot leak the document.
func (c Customer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", c.ID),
		slog.String("tenant", c.TenantID),
	)
}

// MaskedDocument is what a screen may show. The full value only leaves the
// database for whoever passed a policy check that says so.
func (c Customer) MaskedDocument() string {
	if len(c.Document) < 4 {
		return "***"
	}
	return "***" + c.Document[len(c.Document)-4:]
}
