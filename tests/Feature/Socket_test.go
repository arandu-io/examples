package feature_test

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/hesape/config"

	"github.com/arandu-io/examples/bootstrap"
)

// The socket is opened the way a browser opens it: over a real connection, to
// the handler the kernel serves, with every global middleware in the way.
//
// That is the whole point of these tests, and the reason they cannot use the
// in-process client the rest of this suite uses. An upgrade takes the
// connection away from net/http, and a recorder has no connection to take: a
// test that called joaju's handler directly would pass over a pipeline that
// hides the connection from it, which is the failure this route had. Every
// writer the pipeline wraps -- middleware.Observe always, and under
// APP_ENV=dev the live-reload recorder as well -- stands between the handler
// and the connection, and the handshake answers 101 only if the upgrader can
// reach through all of them.

// handshakeKey is the Sec-WebSocket-Key of every handshake below. Any 16 bytes
// in base64 will do; these are the ones RFC 6455 section 1.3 uses, so the
// expected accept value can be checked against the RFC as well as computed.
const handshakeKey = "dGhlIHNhbXBsZSBub25jZQ=="

// acceptFor is the Sec-WebSocket-Accept a server must answer a key with.
func acceptFor(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// openSocket dials the server and sends one handshake for the socket route,
// carrying the cookies given. It returns the connection, a reader positioned
// after the response headers, and the response.
func openSocket(t *testing.T, server *httptest.Server, cookies []*http.Cookie) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dialing the test server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	request, err := http.NewRequest(http.MethodGet,
		server.URL+"/app/"+bootstrap.SocketAppKey+"?protocol=7&client=js&version=8.4.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Sec-WebSocket-Key", handshakeKey)
	request.Header.Set("Sec-WebSocket-Version", "13")
	// The page that opens the socket is served by this host, so the browser
	// names this host as the origin. The upgrader refuses any other.
	request.Header.Set("Origin", server.URL)
	for _, c := range cookies {
		request.AddCookie(c)
	}
	if err := request.Write(conn); err != nil {
		t.Fatalf("writing the handshake: %v", err)
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		t.Fatalf("reading the handshake response: %v", err)
	}
	return conn, reader, response
}

// readTextFrame reads one unfragmented, unmasked text frame: what a server
// sends a client.
func readTextFrame(t *testing.T, r *bufio.Reader) string {
	t.Helper()

	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		t.Fatalf("reading the first frame: %v", err)
	}
	if head[0] != 0x81 {
		t.Fatalf("the first frame starts with %#x, want 0x81 (a final text frame)", head[0])
	}
	if head[1]&0x80 != 0 {
		t.Fatal("the server masked its frame, which RFC 6455 forbids")
	}
	length := uint64(head[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatal(err)
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatal(err)
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > 1<<16 {
		t.Fatalf("the first frame claims %d bytes", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("reading the first frame's payload: %v", err)
	}
	return string(payload)
}

// TestASignedInReaderOpensASocketThroughTheWholePipeline.
//
// A reader signs in through the form, and the cookie the browser now holds
// opens a socket: 101, the accept value RFC 6455 computes from the key, and
// the first frame of the Pusher protocol on the connection that came back.
//
// In both environments, because they build different pipelines: development
// adds the live-reload recorder around every response, and production does
// not. The route carries no middleware of its own to reach the connection, so
// a wrapper anywhere in either pipeline that hid it would answer 500 here.
func TestASignedInReaderOpensASocketThroughTheWholePipeline(t *testing.T) {
	for _, env := range []config.Env{config.EnvDev, config.EnvProd} {
		t.Run(string(env), func(t *testing.T) {
			sqliteEnv(t)
			t.Setenv("CACHE_STORE", "memory")
			t.Setenv("SESSION_DRIVER", "memory")
			t.Setenv("REDIS_URL", "")
			if err := bootstrap.Dispatch("migrate", nil); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			// After the migration and not before: outside development migrate
			// refuses a lock no other replica can see, and the schema is not what
			// this test is about.
			t.Setenv("APP_ENV", string(env))
			app := bootedInstance(t)

			const (
				email    = "socket@example.test"
				password = "a-long-enough-password"
			)
			if _, err := app.Users.Register(context.Background(), bootstrap.Tenant(), "Ana", email, password); err != nil {
				t.Fatalf("registering: %v", err)
			}
			cookies := signInOn(t, app.Kernel.Handler(), email, password)

			server := httptest.NewServer(app.Kernel.Handler())
			t.Cleanup(server.Close)

			_, reader, response := openSocket(t, server, cookies)
			if response.StatusCode != http.StatusSwitchingProtocols {
				body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
				t.Fatalf("the handshake answered %d, want 101. Body: %q", response.StatusCode, body)
			}
			if got, want := response.Header.Get("Sec-WebSocket-Accept"), acceptFor(handshakeKey); got != want {
				t.Fatalf("Sec-WebSocket-Accept = %q, want %q", got, want)
			}
			if want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="; acceptFor(handshakeKey) != want {
				t.Fatalf("the accept value for the RFC's key is %q, want the RFC's %q", acceptFor(handshakeKey), want)
			}

			if frame := readTextFrame(t, reader); !strings.Contains(frame, "pusher:connection_established") {
				t.Fatalf("the first frame on the socket is %q, want pusher:connection_established", frame)
			}
		})
	}
}

// TestAVisitorWithoutASessionIsRefusedTheSocket.
//
// The other half of what the route's middleware is for: with no session there
// is no subject on the request, and joaju answers 401 before anything is
// upgraded. It is what makes the 101 above a statement about a signed-in
// reader rather than about anybody who asks.
func TestAVisitorWithoutASessionIsRefusedTheSocket(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "memory")
	t.Setenv("REDIS_URL", "")
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := bootedInstance(t)

	server := httptest.NewServer(app.Kernel.Handler())
	t.Cleanup(server.Close)

	_, _, response := openSocket(t, server, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a handshake with no session answered %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}
