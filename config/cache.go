package config

import (
	"fmt"
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
)

// CacheStore is which backend the cache runs on.
type CacheStore string

// The supported stores. Two, and they are not two ways to do one thing: memory
// is right for one process and wrong for two, and the interface is the same --
// an adapter behind an interface is not a mode.
const (
	// CacheMemory keeps entries in the process. Right for development and for a
	// single replica; behind a load balancer, half the requests miss.
	CacheMemory CacheStore = "memory"
	// CacheRedis speaks RESP, which is Dragonfly, Redis, Valkey or KeyDB.
	CacheRedis CacheStore = "redis"
)

// Cache is where cached entries are kept, under which prefix, and for how long.
type Cache struct {
	Store CacheStore

	// URL is the RESP endpoint. It is empty when REDIS_URL names none, and only
	// then.
	//
	// It is read whether or not Store is the RESP one, because the cache is not
	// the only thing that names that store: a deployment whose sessions are
	// shared and whose cache stays in each process is a coherent one, and a
	// store that existed only while the cache happened to default to it could
	// not be named by anything else.
	//
	// One reader, here. Every feature that needs the endpoint reads this field
	// rather than REDIS_URL, because two readers of one variable are two answers
	// the day one of them grows a default the other has not.
	URL string

	// Prefix is prepended to every key. It carries the application name so two
	// deployments can share one server without reading each other's entries.
	//
	// It is not the tenant: the tenant is prepended per entry, from the Grant,
	// and never from configuration.
	Prefix string

	// TTL is how long an entry lives when the caller states no lifetime.
	TTL time.Duration
}

func loadCache(base bootstrap.Configuration) (Cache, error) {
	store := CacheStore(env("CACHE_STORE", string(CacheMemory)))
	// The endpoint is read here, and here only. It names the store the cache and
	// the session store of this application can both point at, and nothing in
	// the framework opens a connection to it.
	url := env("REDIS_URL", "")
	switch store {
	case CacheMemory:
	case CacheRedis:
		if url == "" {
			return Cache{}, fmt.Errorf("CACHE_STORE %q requires REDIS_URL", store)
		}
	default:
		return Cache{}, fmt.Errorf("CACHE_STORE has unsupported value %q; expected memory or redis", store)
	}
	ttl, err := envSeconds("CACHE_TTL", 10*time.Minute)
	if err != nil {
		return Cache{}, err
	}
	return Cache{
		Store:  store,
		URL:    url,
		Prefix: env("CACHE_PREFIX", base.App.Name+":cache:"),
		TTL:    ttl,
	}, nil
}
