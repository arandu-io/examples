// Rendered by aru model:build for models.Category. Everything outside the custom block is rewritten on the next build.

package factories

import (
	"context"
	"strings"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"
	factory "github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/examples/app/Models"
)

// CategoryFactory builds rows of categories, for tests and for seeding.
//
// Make builds rows and stores nothing. Create stores them, and takes the Grant
// every write takes: the tenant comes off it, and a factory is no way around the
// policy that guards the table.
//
// Every method returns a new factory, so a factory kept in a variable is never
// changed by a caller that adds a state to it.
type CategoryFactory struct{ f *factory.Factory }

// Categories returns the factory of categories over db, with defineCategory as
// its default state.
//
//	rows, err := factories.Categories(db).Count(10).Create(ctx, g)
//	one, err := factories.Categories(db).State(func(ca *models.Category) { ... }).MakeOne()
//
// The values come from a seeded faker -- the same rows on every run, so a
// failure reproduces -- and Seed asks for others. The key is left empty: the
// model draws a fresh one for every row it stores, so two batches never share
// one, and Make builds rows that have none yet.
func Categories(db model.DB) *CategoryFactory {
	return &CategoryFactory{f: factory.New(models.Categories(db).Base(), func(f faker.Faker, row model.Entity) {
		*row.(*models.Category) = defineCategory(f)
	})}
}

// Count returns a factory that makes n rows.
func (x *CategoryFactory) Count(n int) *CategoryFactory { return &CategoryFactory{f: x.f.Count(n)} }

// Seed returns a factory whose values start from seed.
func (x *CategoryFactory) Seed(seed int64) *CategoryFactory {
	return &CategoryFactory{f: x.f.Seed(seed)}
}

// State returns a factory that applies fn to every row after the default state.
func (x *CategoryFactory) State(fn func(*models.Category)) *CategoryFactory {
	return &CategoryFactory{f: x.f.State(func(row model.Entity) { fn(row.(*models.Category)) })}
}

// Sequence returns a factory that cycles through states, one per row.
func (x *CategoryFactory) Sequence(states ...func(*models.Category)) *CategoryFactory {
	adapted := make([]func(model.Entity), len(states))
	for i, state := range states {
		adapted[i] = func(row model.Entity) { state(row.(*models.Category)) }
	}
	return &CategoryFactory{f: x.f.Sequence(adapted...)}
}

// AfterMaking returns a factory that runs fn on each row once it is built.
func (x *CategoryFactory) AfterMaking(fn func(*models.Category)) *CategoryFactory {
	return &CategoryFactory{f: x.f.AfterMaking(func(row model.Entity) { fn(row.(*models.Category)) })}
}

// AfterCreating returns a factory that runs fn on each row once it is stored.
func (x *CategoryFactory) AfterCreating(fn func(context.Context, auth.Grant, *models.Category) error) *CategoryFactory {
	return &CategoryFactory{f: x.f.AfterCreating(func(ctx context.Context, g auth.Grant, row model.Entity) error {
		return fn(ctx, g, row.(*models.Category))
	})}
}

// Make returns the rows without storing any of them.
func (x *CategoryFactory) Make() (models.CategoryCollection, error) {
	rows, err := x.f.Make()
	return x.collection(rows), err
}

// MakeOne returns one row without storing it, whatever Count says.
func (x *CategoryFactory) MakeOne() (*models.Category, error) {
	e, err := x.f.MakeOne()
	row, _ := e.(*models.Category)
	return row, err
}

// Create stores the rows and returns them.
func (x *CategoryFactory) Create(ctx context.Context, g auth.Grant) (models.CategoryCollection, error) {
	rows, err := x.f.Create(ctx, g)
	return x.collection(rows), err
}

// CreateOne stores one row and returns it, whatever Count says.
func (x *CategoryFactory) CreateOne(ctx context.Context, g auth.Grant) (*models.Category, error) {
	e, err := x.f.CreateOne(ctx, g)
	row, _ := e.(*models.Category)
	return row, err
}

// collection converts the rows the core factory returns. It is a method of
// its own so that Create, which spends a Grant, calls nothing but the core.
func (x *CategoryFactory) collection(rows model.Rows) models.CategoryCollection {
	return models.CategoryCollectionOf(rows)
}

// arandu:begin custom
// Section is the state that names one section outright.
//
// A seeder writes the four sections this example ships with, and they are
// content rather than sample data: a factory that invented twelve random names
// would produce a navigation bar that is mostly dead ends. So the shape comes
// from the definition and the words come from here, which is the division the
// factory is for.
func Section(name, slug, description string) func(*models.Category) {
	return func(ca *models.Category) {
		ca.Name = name
		ca.Slug = slug
		ca.Description = description
	}
}

// Slug is the address half of a name: lowercased, with the spaces turned into
// hyphens.
//
// It is here rather than in the definition because the seeder's states set a
// name and want the matching slug, and two spellings of "what a name looks like
// in a URL" is one spelling too many.
func Slug(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

// defineCategory is the default state: every row the factory makes starts here,
// and a State changes the part a test cares about. It answers a models.Category
// and a state takes a *models.Category, so a column the struct does not declare
// does not compile, where the same mistake against a map of strings is a key
// that is silently dropped and a row that is quietly wrong.
//
// # Make and Create are two different promises
//
//	made, err := factories.Categories(db).MakeOne()          // no Grant, no statement
//	stored, err := factories.Categories(db).CreateOne(ctx, g) // a Grant, and the tenant off it
//
// Make touches nothing, so a Grant on it would authorize nothing, and a
// parameter that looks like enforcement and enforces nothing teaches the
// opposite of what the Grant means everywhere else. Create writes, so it takes
// one, and the tenant of every row it stores is auth.Tenant(g) -- written over
// whatever the definition put in the field. A factory is not a way around the
// policy that guards the table, which is the whole reason the two signatures
// differ.
//
// The rows are built through the model, and that is what lets a made row be
// saved afterwards: the value Make hands back carries the model that would store
// it, rather than being a struct literal with no connection behind it. The
// definition returns the whole struct, and the factory keeps the model the row
// was built with under it.
//
// # The identifier is the faker's, and that is deliberate
//
// The generated comment on Categories says the key is left empty for the model
// to draw. Not on this table: its key is a text identifier the application
// writes -- ManualKey on categoryTable -- so the model draws none, and the
// definition has to supply one. It supplies a reproducible one: the same seed
// answers the same rows, which is what makes a factory failure something a
// second run can reproduce. That also makes it guessable, which is why it is
// fake data for seeds and tests and never the route a request takes to create a
// section. database.NewID is that route, and it is in CategoryService.
func defineCategory(f faker.Faker) models.Category {
	name := strings.ToUpper(f.Word()[:1]) + f.Word()[1:]
	return models.Category{
		ID:          f.UUID(),
		Name:        name,
		Slug:        Slug(name),
		Description: f.Sentence(8),
	}
}

// arandu:end custom
