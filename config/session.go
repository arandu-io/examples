package config

import (
	"fmt"
	"time"
)

// SessionDriver is the cache store session state is kept in.
//
// It names a store rather than inheriting the cache's, so a deployment can
// share its sessions while caching inside each process. The two settings are
// independent on purpose: what the cache loses to a restart is work, and what
// the sessions lose is everybody who was signed in.
type SessionDriver string

// The supported drivers. Same contract, same code path: swapping them is one
// line in bootstrap and no change anywhere else.
const (
	// SessionMemory keeps sessions in the process. Right for one instance and
	// wrong for two: behind a load balancer, half the requests land on the
	// replica that never saw the login.
	SessionMemory SessionDriver = "memory"
	// SessionKV keeps them over RESP, shared by every replica. It is the store
	// CACHE_STORE spells redis; these two words name one store, and the
	// bootstrap is where that is written down once.
	SessionKV SessionDriver = "kv"
)

// Session is where session state is kept and how long a CSRF token lasts.
//
// The two values the session store itself takes are not here. The framework's
// loader reads both into Config.Framework.Session: SESSION_LIFETIME, in
// minutes, into Lifetime, and SESSION_SECURE_COOKIE into Secure. Secure is what
// the session store, the CSRF guest cookie and the flash cookie are all built
// with: three cookies that disagreed about it would be a session that works and
// a form that answers 419, or the other way round.
//
// Nothing else about the cookie is configurable, and that is the store rather
// than an omission here. It writes the cookie on path /, for the host that
// answered, with SameSite=Lax, under a name the CSRF token is bound to. The
// framework's loader refuses SESSION_PATH, SESSION_DOMAIN and
// SESSION_SAME_SITE when they ask for anything else, so this application reads
// none of them: a field that held one would be a value nothing built a cookie
// from.
type Session struct {
	Driver SessionDriver

	// CSRFTTL is how long a CSRF token stays valid: two hours by default, which
	// is never longer than the framework's default session and is a sixth of
	// the twelve hours .env.example writes. Short on purpose: a token that
	// outlives the page it was rendered on is a token that can be replayed.
	CSRFTTL time.Duration
}

// loadSession reads the session settings, against the cache stores that are
// already defined.
//
// It takes the cache rather than reading REDIS_URL a second time. The endpoint
// has one reader -- loadCache -- and a driver that names a store the cache
// configuration did not define is refused here, at the boot, rather than at the
// first request that finds no session where one was written.
func loadSession(cache Cache) (Session, error) {
	driver := SessionDriver(env("SESSION_DRIVER", string(SessionMemory)))
	switch driver {
	case SessionMemory:
	case SessionKV:
		if cache.URL == "" {
			return Session{}, fmt.Errorf("SESSION_DRIVER %q requires REDIS_URL", driver)
		}
	default:
		return Session{}, fmt.Errorf("SESSION_DRIVER has unsupported value %q; expected memory or kv", driver)
	}
	// SESSION_SECURE is retired and refused rather than ignored, for the
	// reason a retired MAIL_ variable is: SESSION_SECURE=false written for a
	// deployment served over http would otherwise be dropped in silence, and
	// the first sign would be every session disappearing between two
	// requests. SESSION_COOKIE is not refused, though nothing reads it either:
	// every .env copied from an older .env.example carries
	// SESSION_COOKIE=arandu_session, a line that never changed anything.
	if env("SESSION_SECURE", "") != "" {
		return Session{}, fmt.Errorf("SESSION_SECURE is retired; remove it. " +
			"SESSION_SECURE_COOKIE decides the Secure attribute: set it to false only to serve over http outside APP_ENV=dev")
	}
	// CSRF_TTL is read here, in seconds, like every other duration in this
	// directory. SESSION_LIFETIME is not: the framework's loader reads it, in
	// minutes, and refuses a SESSION_TTL left over from when this application
	// read the lifetime itself, so a second reader here would be a second answer
	// to how long a login lasts.
	csrfTTL, err := envSeconds("CSRF_TTL", 2*time.Hour)
	if err != nil {
		return Session{}, err
	}
	return Session{Driver: driver, CSRFTTL: csrfTTL}, nil
}
