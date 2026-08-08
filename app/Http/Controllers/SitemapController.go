package controllers

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	services "github.com/arandu-io/examples/app/Services"
)

// SitemapController answers /sitemap.xml.
//
// It is built from the route table and the published posts rather than from a
// list somebody maintains. A sitemap written by hand is a sitemap that is wrong
// one release after it is written, and wrong in the direction nobody notices:
// it keeps listing the page that moved.
//
// # Why it holds a system grant
//
// A crawler has no session, and security.Authorize refuses an anonymous subject
// before it consults a policy. So the choice is between a sitemap that is empty
// for everybody and a named, single-action grant here.
//
// It is the same shape the seeder uses, and it is deliberately narrow: one
// action, one query, and only the fields a sitemap carries -- a slug and a date,
// both of which are on the page it points at. `aru doctor` reports this call
// outside a seeder, a job or a command, and this is the exception the marker
// below declares out loud rather than works around.
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
func (c *SitemapController) Index(ctx *httpx.Context) error {
	//arandu:system-grant a crawler has no session, and a sitemap that needs one lists nothing
	g := security.SystemGrant(policies.PostList, c.tenant)

	found, err := c.posts.ListWith(ctx.Ctx(), g, data.Query{Limit: 1000})
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
		// A draft is not a page yet. Listing one is asking a crawler to index a
		// redirect, and then to keep asking for it.
		if p.PublishedAt.IsZero() {
			continue
		}
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
