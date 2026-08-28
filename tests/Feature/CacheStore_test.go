package feature_test

import (
	"strings"
	"testing"

	"github.com/arandu-io/examples/bootstrap"
)

// CACHE_STORE named a store and the wiring built none.
//
// It parsed, it validated, it refused to be set to a word nobody recognised --
// and no line of the wiring read it, so every deployment cached in the process
// whatever it said. The store is named in the wiring now, and a name nothing
// defines is refused at the boot, where the setting that asked for it can be
// printed, rather than at the first request that finds an empty cache.

// TestACacheStoreThisApplicationCannotBuildIsRefusedAtTheBoot.
//
// The RESP store is the one this application cannot define, and CACHE_STORE=redis
// is how somebody asks for it. The refusal has to name what is missing: a
// message that only said the store was undefined would send whoever reads it
// looking for a typo in their own configuration.
func TestACacheStoreThisApplicationCannotBuildIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:6379")

	cfg, db, _ := openForTest(t)

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started on a cache store nothing in this binary can build")
	} else {
		for _, want := range []string{`"redis"`, "REDIS_URL", "hesape/redis"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %s, and whoever reads it has to guess what is missing: %v", want, err)
			}
		}
	}
}

// TestAnUndefinedCacheStoreIsRefusedAtTheBoot.
//
// The other half of the same door. Load refuses a CACHE_STORE it does not
// recognise, and a configuration assembled in Go skips Load entirely -- which is
// every test in this repository. The refusal that matters is the one in the
// wiring, because it is the one nothing can go around, and it must name the
// store rather than falling back to the one that happens to exist.
func TestAnUndefinedCacheStoreIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	cfg, db, _ := openForTest(t)
	cfg.Cache.Store = "file"

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started on a cache store it does not define, and cached in one it was not asked for")
	} else if !strings.Contains(err.Error(), `"file"`) {
		t.Errorf("the refusal does not name the store that was asked for: %v", err)
	}
}

// TestTheStoreCACHESTORENamesIsTheOneTheApplicationBootsOn.
//
// The positive half, and it is here so the two refusals above cannot be
// satisfied by a wiring that refuses everything.
func TestTheStoreCACHESTORENamesIsTheOneTheApplicationBootsOn(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	cfg, db, _ := openForTest(t)

	if _, err := bootstrap.Build(cfg, db); err != nil {
		t.Fatalf("the in-process store is defined by this application and the boot refused it: %v", err)
	}
}
