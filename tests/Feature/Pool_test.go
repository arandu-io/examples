package feature_test

import (
	"context"
	"testing"
	"time"
)

// The connection pool, asked of the pool rather than of the configuration that
// was supposed to reach it.
//
// config/database.go reads DB_MAX_OPEN_CONNS, DB_MAX_IDLE_CONNS and
// DB_CONN_MAX_LIFETIME, refuses a value that is not a number, and stores all
// three. bootstrap.Open is the single place that carries them onto the
// connection, and there is nothing in between for a reader to inspect -- which
// is what makes the failure quiet when that step is missing: three variables
// are read, the boot succeeds, every value is individually correct, and the
// pool holds whatever the adapter would have held with no configuration at all.
//
// sql.DB.Stats is the witness, and on the engine this suite runs on it answers
// about one of the three. MaxOpenConnections comes back as one whatever was
// configured, because SQLite serializes writes and the adapter pins it there;
// the idle count is not a field of DBStats at all. The lifetime is the one that
// can be asked, and MaxLifetimeClosed is the counter that answers it.

// TestTheConfiguredConnectionLifetimeReachesThePool.
//
// A connection retired for age is one the pool would still be holding if the
// configured lifetime had not arrived, so the counter is the proof and not a
// proxy for it: with the variable dropped on the way to the adapter, the pool
// runs on the default hour and this counter stays at zero for as long as any
// test is willing to wait.
func TestTheConfiguredConnectionLifetimeReachesThePool(t *testing.T) {
	sqliteEnv(t)
	// One second is the shortest this configuration accepts -- it is read as a
	// whole number of seconds and refuses zero -- and the distance between it
	// and the hour an unconfigured pool holds is what the assertion rests on.
	t.Setenv("DB_CONN_MAX_LIFETIME", "1")

	_, db, _ := openForTest(t)
	pool := db.Unwrap()

	// Open connects and pings before it returns, so the pool is already holding
	// one idle connection, and it is older than the lifetime once this returns.
	time.Sleep(1500 * time.Millisecond)

	// The retirement happens where a connection is taken or swept, never on a
	// pool nobody is using, so the query is part of the measurement.
	var one int
	if err := db.QueryRowContext(context.Background(), `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("querying: %v", err)
	}

	// Polled rather than read once: the connection is retired either by the
	// caller that finds it expired or by the sweeper that runs beside the pool,
	// and which of the two gets there first is not something a test should
	// depend on.
	deadline := time.Now().Add(5 * time.Second)
	for pool.Stats().MaxLifetimeClosed == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no connection was retired for age: the pool is running on the adapter's default lifetime rather than the one second this application configured")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTheSQLitePoolIsBoundedToOneWriterWhateverIsConfigured.
//
// This is why the test above asks about the lifetime and not about the size.
// The number configured here is deliberately far above the default, and the
// pool still comes back with one: SQLite gets a single writer, so a larger pool
// would not open a second one -- it would turn the wait into "database is
// locked", which reads like corruption and is really a pool setting.
//
// The bound itself is worth asserting. What database/sql does with no setting
// at all is an unbounded pool, and that is the shape this application must
// never be in on any engine.
func TestTheSQLitePoolIsBoundedToOneWriterWhateverIsConfigured(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "64")

	_, db, _ := openForTest(t)

	if inFlight := db.Unwrap().Stats().MaxOpenConnections; inFlight != 1 {
		t.Fatalf("the pool allows %d connections in flight, and SQLite gets one writer", inFlight)
	}
}
