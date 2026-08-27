package config

import (
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/database"
)

// Database is the connection this application opens, and how its pool behaves.
//
// The connection itself is not re-read here. The framework already parsed
// DATABASE_URL, validated it and built the DSN, and a second parse in the
// application would be a second answer to the same question. This struct
// carries that value and adds what only the application decides: how big the
// pool is and how long a connection lives.
type Database struct {
	// Connection is the engine and its credentials, as the framework parsed
	// them. Hand it to the adapter; never build a DSN by hand.
	Connection database.Config

	// MaxOpenConns caps the connections in flight. Zero is the adapter's
	// default, never an unbounded pool: there is no way to ask for one, which
	// is deliberate, because an unbounded pool is how a burst of traffic turns
	// into "too many connections" on the server and takes every other
	// application with it. SQLite ignores the number and gets a single writer.
	MaxOpenConns int

	// MaxIdleConns is how many stay open between queries. Zero is the adapter's
	// default, and a number above the open limit is the open limit.
	MaxIdleConns int

	// ConnMaxLifetime retires a connection after this long. Zero is the
	// adapter's default. A managed database that rotates behind a proxy hands
	// out connections that stop working, and a lifetime is what makes the pool
	// notice without a failed request.
	ConnMaxLifetime time.Duration
}

func loadDatabase(base bootstrap.Configuration) (Database, error) {
	maxOpenConns, err := envInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return Database{}, err
	}
	maxIdleConns, err := envInt("DB_MAX_IDLE_CONNS", 5)
	if err != nil {
		return Database{}, err
	}
	connMaxLifetime, err := envSeconds("DB_CONN_MAX_LIFETIME", time.Hour)
	if err != nil {
		return Database{}, err
	}
	return Database{
		Connection:      base.Database,
		MaxOpenConns:    maxOpenConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
	}, nil
}
