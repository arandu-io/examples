// Package models holds this application's domain types.
package models

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"
	"github.com/arandu-io/hesape/database/query"
)

// User is an account owned by this application.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with Users(db).New(): a struct literal
// has no connection and its write methods return model.ErrUnwired.
//
// Users, UserQuery and UserCollection are generated beside this file, in
// UserQuery.go, by aru model:build.
type User struct {
	model.Model

	ID       string `db:"id"`
	TenantID string `db:"tenant_id"`
	Name     string `db:"name"`
	Email    string `db:"email"`
	Password string `db:"password"`

	// Roles is the decoded application value. RoleData is the portable JSON
	// text stored in the database; services keep both representations aligned.
	Roles    []string `db:"-"`
	RoleData string   `db:"roles"`

	VerifiedAt *time.Time `db:"verified_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

// The roles this application recognises. Strings, because that is what the
// column holds and what a Policy compares against.
const (
	// RoleAdmin may administer the tenant.
	RoleAdmin = "admin"
	// RoleMember is an ordinary account.
	RoleMember = "member"
)

// userTable is the table User is a row of: the application-owned users table.
//
// ManualKey because the primary key is text the service writes before the
// insert, so it neither increments nor is drawn by the model. The table has no
// updated_at column. The tenant scope is left at its tenant_id default.
var userTable = model.NewTable(model.TableSpec{
	Name:            "users",
	New:             func() model.Entity { return new(User) },
	ManualKey:       true,
	UpdatedAtColumn: model.NoColumn,
})

// DecodeRoles reads the portable JSON column into Roles.
func (u *User) DecodeRoles() error {
	u.Roles = nil
	if u.RoleData == "" {
		u.Roles = []string{}
		return nil
	}
	return json.Unmarshal([]byte(u.RoleData), &u.Roles)
}

// EncodeRoles returns Roles as portable JSON text.
func (u User) EncodeRoles() (string, error) {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	b, err := json.Marshal(roles)
	return string(b), err
}

// Verified reports whether the address was confirmed.
func (u User) Verified() bool { return u.VerifiedAt != nil && !u.VerifiedAt.IsZero() }

// Subject returns the session subject derived only from stored account data.
func (u User) Subject() auth.Subject {
	return auth.Subject{
		ID: u.ID, Tenant: u.TenantID, Roles: append([]string(nil), u.Roles...), Verified: u.Verified(),
	}
}

// PasswordFingerprint identifies the complete current password hash without
// exposing that hash in a signed URL or pending cookie.
func (u User) PasswordFingerprint() string {
	sum := sha256.Sum256([]byte(u.Password))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// MarshalJSON keeps password material and its persistence representation out
// of responses and dumps.
func (u User) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID         string     `json:"id"`
		TenantID   string     `json:"tenant_id"`
		Name       string     `json:"name"`
		Email      string     `json:"email"`
		Roles      []string   `json:"roles"`
		VerifiedAt *time.Time `json:"verified_at,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
	}{
		ID: u.ID, TenantID: u.TenantID, Name: u.Name, Email: u.Email,
		Roles: append([]string(nil), u.Roles...), VerifiedAt: u.VerifiedAt, CreatedAt: u.CreatedAt,
	})
}

// LogValue records only identifiers when a whole User reaches structured logs.
func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", u.ID), slog.String("tenant", u.TenantID))
}

// arandu:begin custom
// Local scopes are methods on *UserQuery, relations are registered on
// userTable in an init function, and anything else about this entity goes
// here too.

// GetQuery returns the statement under the query, without the model around
// it: the users table and the clauses added so far, whose rows come back as
// query.Record. It is what the native user provider reads accounts through,
// because it fills the account it hands to the guard itself. Its terminals
// take a Grant and are filtered by the Grant's tenant, like the model's.
func (q *UserQuery) GetQuery() *query.Builder { return q.Base().GetQuery() }

// arandu:end custom
