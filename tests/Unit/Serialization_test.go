package unit_test

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	models "github.com/arandu-io/examples/app/Models"
)

// The entities, filled in, so that a field which stopped being written is a
// field that stopped appearing rather than one that happened to be empty.
func filledModels() (models.Post, models.Comment, models.Category) {
	at := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	return models.Post{
			ID: "p1", TenantID: "t1", Title: "A title", Slug: "a-title",
			Body: "A body.", CategoryID: "ca1", Views: 7,
			PublishedAt: at, CreatedAt: at,
		},
		models.Comment{
			ID: "c1", TenantID: "t1", PostId: "p1", Author: "u1",
			Body: "A remark.", Approved: true, CreatedAt: at,
		},
		models.Category{
			ID: "ca1", TenantID: "t1", Name: "News", Slug: "news",
			Description: "What happened.", CreatedAt: at,
		}
}

// TestEveryModelNamesTheFieldsThatMayLeave.
//
// Each entity writes its JSON by naming its fields one at a time, so a column
// added to the struct and not named there is private by omission -- which is
// what a database default is not. That guarantee is only worth having if
// something notices when the method stops naming them, and until this test
// existed nothing did: replacing all three methods with a plain marshal of the
// struct left the whole suite green, and every field went out under its Go name
// instead.
//
// The assertion is the key set and not a rendered document. Comparing whole JSON
// would fail on a value somebody changed in a fixture, and a test that fails for
// two reasons is one people stop reading.
func TestEveryModelNamesTheFieldsThatMayLeave(t *testing.T) {
	post, comment, category := filledModels()

	for _, c := range []struct {
		what  string
		value any
		names []string
	}{
		{"post", post, []string{
			"body", "category_id", "created_at", "id", "published_at",
			"slug", "tenant_id", "title", "views",
		}},
		{"comment", comment, []string{
			"approved", "author", "body", "created_at", "id", "post_id", "tenant_id",
		}},
		{"category", category, []string{
			"created_at", "description", "id", "name", "slug", "tenant_id",
		}},
	} {
		t.Run(c.what, func(t *testing.T) {
			body, err := json.Marshal(c.value)
			if err != nil {
				t.Fatalf("marshalling a %s: %v", c.what, err)
			}

			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatalf("a %s did not serialize as an object: %v", c.what, err)
			}

			got := make([]string, 0, len(fields))
			for name := range fields {
				got = append(got, name)
			}
			sort.Strings(got)

			if strings.Join(got, ",") != strings.Join(c.names, ",") {
				t.Fatalf("the %s allow-list changed.\n want: %s\n  got: %s\n"+
					"A field appears here only because MarshalJSON names it. If one arrived "+
					"without being decided about, decide about it there.",
					c.what, strings.Join(c.names, ","), strings.Join(got, ","))
			}
		})
	}
}

// TestEveryModelLogsItsIdentifiersAndNothingElse.
//
// LogValue is the other half of the same claim, and it is the half that runs
// without anybody asking: passing an entity to a log call, to observability.Dump
// or to the debug page reaches this method rather than the struct. A whole
// entity written into a log line is a body, an author and an email address in a
// file that outlives the request and is read by more people than the page was.
func TestEveryModelLogsItsIdentifiersAndNothingElse(t *testing.T) {
	post, comment, category := filledModels()

	for _, c := range []struct {
		what  string
		value slog.LogValuer
	}{
		{"post", post},
		{"comment", comment},
		{"category", category},
	} {
		t.Run(c.what, func(t *testing.T) {
			v := c.value.LogValue()
			if v.Kind() != slog.KindGroup {
				t.Fatalf("a %s logs as %s and not as a group: it is writing itself whole", c.what, v.Kind())
			}

			got := make([]string, 0, 2)
			for _, attr := range v.Group() {
				got = append(got, attr.Key)
			}
			sort.Strings(got)

			if strings.Join(got, ",") != "id,tenant" {
				t.Fatalf("a %s logs %s, and the identifiers are all that may go", c.what, strings.Join(got, ","))
			}
		})
	}
}
