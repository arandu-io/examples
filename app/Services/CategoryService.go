package services

import (
	"context"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
)

// Pagination bounds for sections. A request that asks for everything gets the
// maximum, never everything: an unbounded query is how one page load takes a
// database down.
const (
	categoryDefaultLimit = 50
	categoryMaxLimit     = 200
)

// categorySortable is the ordering allowlist. A sort field is a column name, and
// a column name taken from the request is injection through a door nobody
// watches -- the model layer quotes an identifier, it does not judge it.
var categorySortable = map[string]string{
	"":            "created_at",
	"created_at":  "created_at",
	"name":        "name",
	"slug":        "slug",
	"description": "description",
}

// CategoryService holds the business rules, and it is where the statements
// about sections are written. It receives its dependencies through the
// constructor -- explicit wiring, no container.
//
// # Why there is no CategoryRepository under this
//
// There was one, and every statement in it was CRUD over a single table: find
// by id, find by slug, page by a keyset cursor, list, insert, update, delete.
// A repository is where a query too complex to generate lives -- a join, an
// aggregate, a shape built for a screen -- and none of those was that. So the
// CRUD moved to the model and the file went, which is the distinction the
// collection is built on: PostRepository keeps CountByCategory because a count
// per section is a report, and CommentRepository stays whole because its writes
// carry an EXISTS that no single-table statement can express.
//
// What did not move is the authorization. Every method below asks the Policy
// first and runs the statement with the Grant that answer produced. The Grant is
// minted and spent in one function body, which is why there is no second
// g.Check beside it: Check earns its place where a Grant crosses a boundary --
// as it does on the way into a repository, and as it still does in the two that
// remain -- and here it would re-ask, three lines later, a question this
// function asked itself.
//
// The tenant is not asked about at all, and that is the point of the shape: it
// is on the Grant, and the model puts it in the where clause of every statement
// it compiles. A read that lost it does not return the wrong rows, it does not
// run.
type CategoryService struct {
	db     *data.DB
	policy policies.CategoryPolicy
	// posts is the policy consulted before the section relation is read. See
	// Delete: reading another entity is a read like any other, and it is
	// authorized by that entity's policy rather than by the one that allowed
	// the delete.
	posts policies.PostPolicy
}

// NewCategoryService wires the service.
//
// It takes the handle rather than a repository, and no adapter goes in between:
// *data.DB is a model connection, so a statement built here runs on the same
// pool, joins the same open transaction, and is recorded on the same Collector
// as one issued by the repositories next door.
func NewCategoryService(db *data.DB) *CategoryService {
	return &CategoryService{db: db}
}

// Create walks the mandatory path: validate, Authorize, Grant, statement.
// There is no other order that compiles.
func (s *CategoryService) Create(ctx context.Context, actor security.Subject, in requests.StoreCategory) (models.Category, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Category{}, errs
	}

	candidate := models.Category{
		Name:        in.Name,
		Slug:        in.Slug,
		Description: in.Description,
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryCreate, candidate)
	if err != nil {
		return models.Category{}, err
	}

	// The id comes from the application, not from a database default:
	// gen_random_uuid, UUID() and randomblob are three spellings of one idea,
	// and depending on any of them would tie the schema to one engine.
	if candidate.ID, err = data.NewID(); err != nil {
		return models.Category{}, err
	}

	// A row is built by the model and then filled with the struct, rather than
	// by handing the model a map of column names. The map is the shape that
	// drops a key nobody noticed was misspelled; the struct is the one the
	// compiler reads. The tenant is deliberately not set here -- whatever this
	// field holds, the insert writes data.Tenant(g) over it.
	instance, err := models.Categories(s.db).NewInstance(nil, false)
	if err != nil {
		return models.Category{}, err
	}
	*instance.Entity = candidate

	if _, err := instance.Save(ctx, g); err != nil {
		if conflict(err) {
			return models.Category{}, models.ErrCategoryConflict
		}
		return models.Category{}, err
	}
	created := *instance.Entity

	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("category.created", created)
	}
	return created, nil
}

// Get returns one section.
//
// The first Authorize answers whether the caller may look at sections at all.
// The second, with the row that was read, is the object-level decision — a
// policy that branches on the entity's fields is only consulted the second time.
func (s *CategoryService) Get(ctx context.Context, actor security.Subject, id string) (models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return models.Category{}, err
	}

	// The tenant comes from the Grant, never from the request. An id belonging
	// to another tenant matches nothing, so it answers ErrCategoryNotFound
	// rather than the row -- which is the same answer an id that does not exist
	// gets, and telling the two apart is itself a leak.
	found, err := models.Categories(s.db).NewQuery().WhereKey(id).First(ctx, g)
	if err != nil {
		return models.Category{}, err
	}
	if found == nil {
		return models.Category{}, models.ErrCategoryNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, *found); err != nil {
		return models.Category{}, err
	}
	return *found, nil
}

// List returns a page of sections.
//
// Pagination is keyset based: OFFSET grows more expensive with every page and
// skips rows when data changes underneath it. The cursor is the id of the last
// row read, so the value it sorts by has to be looked up before the page can be
// built -- which used to be a correlated subquery that needed a tenant of its
// own, and is now a terminal that cannot run without one.
func (s *CategoryService) List(ctx context.Context, actor security.Subject, q data.Query) ([]models.Category, error) {
	// CategoryList, not CategoryView. The specification can grant them to
	// different roles -- "a support agent may open the record it was given, but
	// may not page through every record there is" -- and checking view here made
	// the list permission decorative: whoever could read one could read all of
	// them.
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryList, models.Category{})
	if err != nil {
		return nil, err
	}

	column, ok := categorySortable[q.Sort]
	if !ok {
		return nil, models.ErrCategorySort
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = categoryDefaultLimit
	case limit > categoryMaxLimit:
		limit = categoryMaxLimit
	}

	sections := models.Categories(s.db)
	page := sections.NewQuery()

	if q.Cursor != "" {
		// The anchor is read through a terminal of its own, and that is what
		// replaced the subquery. A cursor arrives from the client, so an id of
		// another tenant would once have decided where our page starts; here the
		// lookup is a statement the model scopes by data.Tenant(g), so such an
		// id resolves to nothing and the page is empty.
		anchor, err := sections.NewQuery().WhereKey(q.Cursor).Value(ctx, g, column)
		if err != nil {
			return nil, err
		}
		if anchor == nil {
			return nil, nil
		}
		// The predicate names the column the ORDER BY names, and the id breaks
		// the tie: two rows sharing a created_at would otherwise page over each
		// other forever.
		page = page.Where(func(after *model.Builder[models.Category]) {
			after.Where(column, ">", anchor).
				OrWhere(func(equal *model.Builder[models.Category]) {
					equal.Where(column, "=", anchor).Where("id", ">", q.Cursor)
				})
		})
	}

	found, err := page.OrderBy(column).OrderBy("id").Limit(limit).Get(ctx, g)
	if err != nil {
		return nil, err
	}
	return entities(found), nil
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *CategoryService) Update(ctx context.Context, actor security.Subject, in requests.UpdateCategory) (models.Category, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Category{}, errs
	}

	stored, err := s.Get(ctx, actor, in.ID)
	if err != nil {
		return models.Category{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryUpdate, stored)
	if err != nil {
		return models.Category{}, err
	}
	stored.Name = in.Name
	stored.Slug = in.Slug
	stored.Description = in.Description

	// The columns are named rather than derived from the struct, and the tenant
	// is not one of them: moving a row between tenants is not an update, it is a
	// migration. Writing the whole entity back would put tenant_id in the SET
	// list, where a value the caller chose could file the row under somebody
	// else -- the where clause would still find only our row, and the row would
	// leave.
	changed, err := models.Categories(s.db).NewQuery().WhereKey(stored.ID).
		Update(ctx, g, map[string]any{
			"name":        stored.Name,
			"slug":        stored.Slug,
			"description": stored.Description,
		})
	if err != nil {
		if conflict(err) {
			return models.Category{}, models.ErrCategoryConflict
		}
		return models.Category{}, err
	}
	if changed == 0 {
		// Classify rather than conclude. The row was read three lines above,
		// under the same tenant, so zero affected rows is more often a driver
		// that counts changed rows instead of matched ones -- MySQL does, and
		// saving a form without editing anything is what produces it -- than a
		// row that is not there. Answering not-found for that is the bug users
		// meet on their first save.
		//
		// So it is asked again. The window between the read and the write is
		// real, and this is the read that closes it: the row is gone only if it
		// is gone now.
		gone, err := models.Categories(s.db).NewQuery().WhereKey(stored.ID).Count(ctx, g)
		if err != nil {
			return models.Category{}, err
		}
		if gone == 0 {
			return models.Category{}, models.ErrCategoryNotFound
		}
	}
	return stored, nil
}

// Delete removes a section, once it is empty.
//
// The policy allows the action and says, in prose, that a section holding
// articles is emptied first. It cannot enforce that: the count is not on the
// entity, and a policy that queried would be a policy with a data layer under
// it. This is the layer that can, and the guard is the relation.
func (s *CategoryService) Delete(ctx context.Context, actor security.Subject, id string) error {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryDelete, stored)
	if err != nil {
		return err
	}

	// Reading the articles is a read, and it is authorized by the policy of the
	// entity being read rather than by the one that allowed this delete. A
	// second Grant, for post.list, because a Grant carries ONE action: handing
	// the delete Grant to a query over posts would be an authorization nobody
	// asked for.
	listing, err := security.Authorize(ctx, s.posts, actor, policies.PostList, models.Post{})
	if err != nil {
		return err
	}
	filed, err := models.PostsIn(ctx, listing, s.db, stored)
	if err != nil {
		return err
	}
	if len(filed) > 0 {
		return models.ErrCategoryNotEmpty
	}

	removed, err := models.Categories(s.db).NewQuery().WhereKey(id).Delete(ctx, g)
	if err != nil {
		return err
	}
	if removed == 0 {
		return models.ErrCategoryNotFound
	}
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("category.deleted", stored)
	}
	return nil
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.

// BySlug returns the section a reader's URL names.
//
// It asks the two questions Get asks, and for the same reason: it returns one
// row, and the row is chosen by the caller. The first Authorize is permission to
// look at sections at all, which is what produces the Grant the statement needs;
// the second is about the section that came back, and it is where a rule that
// reads the entity's fields is decided.
//
// A lookup by slug rather than by id does not make the second question optional.
// Both take a value from the address bar and answer with one record, and a read
// path that skips the object-level decision is one where a rule written against
// the entity is never consulted.
func (s *CategoryService) BySlug(ctx context.Context, actor security.Subject, slug string) (models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return models.Category{}, err
	}

	// A slug is unique per tenant and not globally, so the tenant is part of the
	// lookup rather than a check on the row that came back -- and it is part of
	// it because the model puts it there, not because this line remembered to.
	// A slug is also the half of the address a reader can type, which makes this
	// the query most likely to be handed another tenant's value.
	found, err := models.Categories(s.db).NewQuery().
		Where("slug", "=", normalize(slug)).First(ctx, g)
	if err != nil {
		return models.Category{}, err
	}
	if found == nil {
		return models.Category{}, models.ErrCategoryNotFound
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, *found); err != nil {
		return models.Category{}, err
	}
	return *found, nil
}

// All returns every section, for the navigation.
//
// No cursor and no limit beyond the bound, deliberately. A blog has a dozen
// sections, the navigation shows all of them, and paging a navigation is a
// navigation that hides part of itself. The bound is the schema: if this ever
// returns hundreds, the sections are being used as tags and the answer is a
// different feature.
func (s *CategoryService) All(ctx context.Context, actor security.Subject) ([]models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryList, models.Category{})
	if err != nil {
		return nil, err
	}
	found, err := models.Categories(s.db).NewQuery().
		OrderBy("name").OrderBy("id").Limit(categoryMaxLimit).Get(ctx, g)
	if err != nil {
		return nil, err
	}
	return entities(found), nil
}

// arandu:end custom

// entities turns what a terminal answers -- a collection of pointers into the
// rows it hydrated -- into the values the rest of this application passes
// around.
//
// The copy is the point rather than a cost. A view struct, a policy argument and
// a template all take the entity by value here, and handing out the pointer the
// query holds would let a template write into the row a policy was asked about.
func entities(found []*models.Category) []models.Category {
	if len(found) == 0 {
		return nil
	}
	out := make([]models.Category, 0, len(found))
	for _, row := range found {
		out = append(out, *row)
	}
	return out
}

// normalize lowercases and trims, which is what keeps a plain UNIQUE index
// case-insensitive on every engine.
func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// conflict recognizes a duplicate key across engines by message, which is the
// price of not importing a driver into a service.
func conflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry")
}
