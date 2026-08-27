package factories

import (
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/examples/app/Models"
)

// Categories returns the factory for sections.
//
// It is the typed one: the definition answers a models.Category, a state takes a
// *models.Category, and what comes back is the entity. A column the struct does
// not declare does not compile, where the same mistake against a map of strings
// is a key that is silently dropped and a row that is quietly wrong.
//
// # Make and Create are two different promises
//
//	made, err := factories.Categories(db).MakeOne()          // no Grant, no statement
//	stored, err := factories.Categories(db).CreateOne(ctx, g) // a Grant, and the tenant off it
//
// Make touches nothing, so a Grant on it would authorize nothing, and a
// parameter that looks like enforcement and enforces nothing teaches the
// opposite of what the Grant means everywhere else. Create writes, so it takes
// one, and the tenant of every row it stores is data.Tenant(g) -- written over
// whatever the definition put in the field. A factory is not a way around the
// policy that guards the table, which is the whole reason the two signatures
// differ.
//
// It takes the handle because a factory builds rows through the model, and that
// is what lets a made row be saved afterwards: the value it hands back carries
// the model that would store it, rather than being a struct literal with no
// connection behind it.
//
// # The identifier is the faker's, and that is deliberate
//
// The key does not increment -- it is a text identifier the application
// generates -- so the definition has to supply one, and it supplies a
// reproducible one: the same seed answers the same rows, which is what makes a
// factory failure something a second run can reproduce. That also makes it
// guessable, which is why it is fake data for seeds and tests and never the
// route a request takes to create a section. data.NewID is that route, and it is
// in CategoryService.
func Categories(db *data.DB) *factories.Factory[models.Category] {
	return factories.For(models.Categories(db), func(f faker.Faker) models.Category {
		name := strings.ToUpper(f.Word()[:1]) + f.Word()[1:]
		return models.Category{
			ID:          f.UUID(),
			Name:        name,
			Slug:        Slug(name),
			Description: f.Sentence(8),
		}
	})
}

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
