package config

import (
	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/database"
)

// Database is the connection this application opens, pool and all.
//
// Nothing here is read from the environment. The framework already parsed
// DATABASE_URL, refused a pool setting that is present and not a number, and
// built the value below. A second reader in the application would be a second
// answer to the same question, and the two would disagree exactly where it
// costs most: on a value that is set and wrong, where one stops the boot and
// the other falls back without saying so.
//
// The three pool settings live inside Connection rather than beside it, and
// that is what keeps their defaults in one place. Zero on any of them means the
// adapter's own default, so the numbers are written once, where the pool is
// applied; an application that repeated them here would hold the old ones on
// the day they changed, with nothing to report it.
type Database struct {
	// Connection is the engine, its credentials and its pool, as the framework
	// parsed them. Hand it to the adapter whole: never build a DSN by hand, and
	// never rebuild the pool on the way.
	Connection database.Config
}

// loadDatabase carries the parsed connection into the application's shape.
//
// It reads nothing and cannot fail, which is the point: everything this domain
// needed to validate was validated before it got here.
func loadDatabase(base bootstrap.Configuration) Database {
	return Database{Connection: base.Database}
}
