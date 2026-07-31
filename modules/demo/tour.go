package demo

import "html/template"

// tour is the index of the demonstration, in plain HTML.
//
// No CSS build, no assets, no framework of any kind: the view layer is phase 2,
// and this page has to work today. It is also a small demonstration in itself --
// the error page next door is styled and needs no build either.
var tour = template.Must(template.New("tour").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>arandu — guided tour</title>
<style>
body{font:16px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;max-width:52rem;margin:3rem auto;padding:0 1.5rem;background:#0d1117;color:#e6edf3}
h1{font-size:1.2rem;color:#58a6ff;margin-bottom:0}
p.sub{color:#8b949e;margin-top:.3rem}
h2{font-size:.75rem;text-transform:uppercase;letter-spacing:.08em;color:#8b949e;border-bottom:1px solid #30363d;padding-bottom:.4rem;margin-top:2.5rem}
a{color:#e6edf3}
li{margin:.9rem 0}
code{color:#d29922}
.why{color:#8b949e;font-size:.9rem;display:block}
</style></head><body>
<h1>arandu — guided tour</h1>
<p class="sub">Every route below makes one claim visible. Sign in first at <a href="/auth/login">/auth/login</a>.</p>

<h2>Debugging is core, not a plugin</h2>
<ul>
  <li><a href="/demo/n-plus-one">/demo/n-plus-one</a>
    <span class="why">A loop that asks for each customer's invoices. The page names the N+1 and points at this loop, not at the repository.</span></li>
  <li><a href="/demo/batched">/demo/batched</a>
    <span class="why">The same page done right: two queries, whatever the number of customers. Compare the <code>queries</code> field in the request log.</span></li>
  <li><a href="/demo/slow-query">/demo/slow-query</a>
    <span class="why">A sum over a column with no index. The diagnosis says which statement to look at.</span></li>
  <li><a href="/demo/dump">/demo/dump</a>
    <span class="why">Values recorded with their origin and their offset into the request. Note the customer document is not there.</span></li>
  <li><a href="/demo/panic">/demo/panic</a>
    <span class="why">A plain failure: stack with your frames open and the framework's collapsed, and the queries that ran before it.</span></li>
</ul>

<h2>Authorization cannot be bypassed</h2>
<ul>
  <li><a href="/demo/other-tenant">/demo/other-tenant</a>
    <span class="why">A real id from another tenant, asked for as yourself. The answer is "not found", not "forbidden" — the row was never a candidate.</span></li>
  <li><a href="/demo/no-grant">/demo/no-grant</a>
    <span class="why">The half that cannot be shown at runtime: without a Grant it does not compile.</span></li>
</ul>

<h2>The application itself</h2>
<ul>
  <li><a href="/customers">/customers</a> <span class="why">The canonical module: entity, policy, repository, service, request, handlers.</span></li>
  <li><a href="/invoices/outstanding">/invoices/outstanding</a> <span class="why">A second module, related to the first.</span></li>
  <li><a href="/_arandu/health">/_arandu/health</a> <span class="why">200 with the database up, 503 without, naming the module that failed.</span></li>
  <li><a href="/_arandu/debug">/_arandu/debug</a> <span class="why">The request console. Phase 3 fills it in.</span></li>
</ul>
</body></html>`))
