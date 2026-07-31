package seeders

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/examples/modules/customer"
	"github.com/arandu-io/examples/modules/invoice"
)

// DemoDataSeeder fills both tenants with customers and invoices.
//
// The volume is chosen, not arbitrary: six customers with several invoices each
// is enough for the N+1 route to cross the Collector's threshold of five
// identical statements. A demonstration that needs the reader to imagine the
// problem has not demonstrated it.
//
// The second tenant exists so /demo/other-tenant fails against real rows rather
// than against an empty table -- an isolation check that passes because there is
// nothing to read proves nothing.
type DemoDataSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (DemoDataSeeder) Name() string { return "DemoDataSeeder" }

// demoCustomer is one row to create, with the invoices that belong to it.
type demoCustomer struct {
	name     string
	email    string
	document string
	invoices []int64 // amounts in cents
}

var mainTenantData = []demoCustomer{
	{"Ferrovia Central", "contato@ferroviacentral.test", "11222333000181", []int64{125_00, 340_50, 89_90}},
	{"Marcenaria Horizonte", "financeiro@horizonte.test", "22333444000172", []int64{1_250_00, 430_00}},
	{"Padaria do Largo", "contato@padariadolargo.test", "33444555000163", []int64{78_40, 78_40, 156_80, 22_10}},
	{"Transportes Aurora", "faturamento@aurora.test", "44555666000154", []int64{3_400_00}},
	{"Clínica Vale Verde", "administrativo@valeverde.test", "55666777000145", []int64{690_00, 690_00, 145_30}},
	{"Oficina Beira-Rio", "contato@beirario.test", "66777888000136", []int64{212_75, 98_00}},
}

var otherTenantData = []demoCustomer{
	{"Concorrente Um", "contato@concorrente-um.test", "77888999000127", []int64{500_00}},
	{"Concorrente Dois", "contato@concorrente-dois.test", "88999000000118", []int64{750_00, 250_00}},
}

// Run creates the rows, and is safe to run twice: an existing email is treated
// as success, because a seeder that fails on the second run cannot be part of a
// deploy.
func (DemoDataSeeder) Run(ctx context.Context, d Deps) error {
	if d.Customers == nil || d.Invoices == nil {
		return errors.New("the customer and invoice services are not wired")
	}
	if d.Tenant == "" || d.OtherTenant == "" {
		return errors.New("both tenants are required: seeding into an empty tenant creates rows nobody can reach")
	}

	created := 0
	for _, tenant := range []struct {
		id   string
		data []demoCustomer
	}{
		{d.Tenant, mainTenantData},
		{d.OtherTenant, otherTenantData},
	} {
		for i, row := range tenant.data {
			c, err := d.Customers.EnsureSeed(ctx, tenant.id, customer.Customer{
				Name:     row.name,
				Email:    row.email,
				Document: row.document,
			})
			if errors.Is(err, customer.ErrEmailTaken) {
				continue // already seeded
			}
			if err != nil {
				return err
			}
			created++

			for n, cents := range row.invoices {
				_, err := d.Invoices.EnsureSeed(ctx, tenant.id, invoice.Invoice{
					CustomerID:  c.ID,
					Number:      fmt.Sprintf("%s-%03d-%02d", shortTenant(tenant.id), i+1, n+1),
					AmountCents: cents,
					Status:      invoice.StatusOpen,
				})
				if err != nil {
					return err
				}
			}
		}
	}

	if created == 0 {
		fmt.Println("demo data already present")
		return nil
	}
	fmt.Printf("created %d customers with their invoices, across two tenants\n", created)
	return nil
}

// shortTenant makes invoice numbers readable without leaking the whole id.
func shortTenant(id string) string {
	if len(id) < 4 {
		return "T"
	}
	return id[len(id)-4:]
}
