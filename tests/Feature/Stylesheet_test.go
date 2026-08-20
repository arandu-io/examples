package feature_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/view"

	"github.com/arandu-io/examples/tests"
)

// TestTheBrowserGetsThisProjectsStylesheet guards a whole pipeline whose output
// nothing is obliged to consume.
//
// `aru view:build` runs Tailwind over resources/css/app.css and writes
// assets/app.css. If nothing embeds it, the browser receives the framework's
// default instead -- byte for byte, same md5 -- and every class written in a
// view of this project is absent from the page, with no error anywhere.
//
// Nothing here imports the stylesheet package. It arrives the way it arrives in
// production, through bootstrap, and that is the point: an import written in
// this file would keep the test green after somebody deleted the one in
// bootstrap, which is the failure worth catching.
//
// The check is the md5 of what comes over HTTP against the md5 of the file on
// disk. Anything weaker passes in the broken state: the framework's default is
// also valid CSS, also served with a 200, and also has Tailwind's banner.
func TestTheBrowserGetsThisProjectsStylesheet(t *testing.T) {
	onDisk, err := os.ReadFile(filepath.Join(tests.Root(t), "assets", "app.css"))
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}

	r := fhttp.NewRouter()
	view.NewModule().Routes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + view.URL(view.Stylesheet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	served, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the stylesheet answered %d", resp.StatusCode)
	}

	if sum(served) != sum(onDisk) {
		t.Errorf("the browser is served a different stylesheet than assets/app.css.\n  served   %s (%d bytes)\n  on disk  %s (%d bytes)\nThis is the framework's default: nothing registered this project's.",
			sum(served), len(served), sum(onDisk), len(onDisk))
	}
}

func sum(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}
