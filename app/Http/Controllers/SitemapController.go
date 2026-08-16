package controllers

import (
	"encoding/xml"
	"net/http"
	"time"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
)

// SitemapController answers /sitemap.xml.
//
// It is built from the route table and the published posts rather than from a
// list somebody maintains. A sitemap written by hand is a sitemap that is wrong
// one release after it is written, and wrong in the direction nobody notices:
// it keeps listing the page that moved.
//
// # It holds no system grant, and that is the point
//
// A crawler has no session, and Authorize refuses an empty subject before it
// consults a policy -- so the shortcut is security.SystemGrant, which skips the
// policy altogether and serves a public page with the instrument a scheduled job
// uses.
//
// This reads as a guest instead, through PostPolicy, and what that buys is not
// tidiness: the sitemap cannot list something the policy would refuse to serve,
// because one rule answers both questions. A system grant would list every
// draft, and the crawler would find a redirect behind each one.
type SitemapController struct {
	Controller

	posts  *services.PostService
	tenant string
	// base is the absolute origin the URLs are built on. A sitemap of relative
	// paths is refused by every crawler that reads one.
	base string
}

// NewSitemapController returns the controller. bootstrap builds it.
func NewSitemapController(posts *services.PostService, tenant, base string) *SitemapController {
	return &SitemapController{posts: posts, tenant: tenant, base: base}
}

// urlset is the document, in the schema crawlers agreed on.
type urlset struct {
	XMLName xml.Name  `xml:"urlset"`
	NS      string    `xml:"xmlns,attr"`
	URLs    []sitemap `xml:"url"`
}

type sitemap struct {
	Location string `xml:"loc"`
	Modified string `xml:"lastmod,omitempty"`
	Priority string `xml:"priority,omitempty"`
}

// Index writes the sitemap.
func (c *SitemapController) Index(ctx *fhttp.Context) error {
	// The same guest a reader is. PostPolicy allows the published listing and
	// refuses PostList, so what comes back is what a visitor is allowed to see.
	found, err := c.posts.Published(ctx.Ctx(), security.Guest(c.tenant), 1000)
	if err != nil {
		return err
	}

	doc := urlset{
		NS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: []sitemap{
			{Location: c.base + ctx.URL("home"), Priority: "1.0"},
			{Location: c.base + ctx.URL("posts.index"), Priority: "0.8"},
		},
	}

	for _, p := range found {
		doc.URLs = append(doc.URLs, sitemap{
			Location: c.base + ctx.URL("posts.show", p.ID),
			Modified: p.PublishedAt.Format(time.DateOnly),
			Priority: "0.6",
		})
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	ctx.Response.Header().Set("Content-Type", "application/xml; charset=utf-8")
	ctx.Response.WriteHeader(http.StatusOK)
	_, err = ctx.Response.Write(append([]byte(xml.Header), body...))
	return err
}

// Compile-time proof that the sitemap reads the entity it claims to.
var _ = models.Post{}
