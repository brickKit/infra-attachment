// Package catalog keeps the copy of the attachment category catalog (docs/design.md AT01) and answers the owner check
// (AT02): which component owns a resource type and which categories it declared.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	core "github.com/brickKit/infra-attachment/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/work"
	authzv3 "github.com/brickKit/contract-infra-authz/v3/gen/go/be/authz/v3"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
)

// JobSync is the catalog's refresh, every minute and at the start.
const JobSync = "sync-catalog"

// GRPC is the authorization member's CatalogService (contract-infra-authz alpha.7).
type GRPC struct{ Conn grpc.ClientConnInterface }

// Fetch reads the catalog; unchanged when known is the current revision.
func (g GRPC) Fetch(ctx context.Context, known string) (string, bool, []core.Category, error) {
	ans, err := authzv3.NewCatalogServiceClient(g.Conn).AttachmentCategories(ctx,
		&authzv3.AttachmentCategoriesRequest{KnownRevision: known})
	if err != nil {
		return "", false, nil, err
	}
	if ans.GetUnchanged() {
		return ans.GetRevision(), true, nil, nil
	}
	var out []core.Category
	for _, r := range ans.GetResources() {
		var cats map[string]core.Category
		if err := json.Unmarshal(r.GetCategoriesJson(), &cats); err != nil {
			return "", false, nil, fmt.Errorf("catalog: %s's categories: %w", r.GetResourceType(), err)
		}
		for name, c := range cats {
			c.ResourceType, c.Owner, c.Name = r.GetResourceType(), r.GetOwnerComponent(), name
			out = append(out, c)
		}
	}
	return ans.GetRevision(), false, out, nil
}

// Sync refreshes the copy (AT01): a changed catalog replaces it whole; one that cannot be read leaves the copy and
// alerts once it is older than CatalogStale.
func Sync(m *core.Base, ctx context.Context, _ *work.Run) error {
	if m.Catalog == nil {
		return nil
	}
	var known string
	var fetched time.Time
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		err := tx.QueryRow(ctx, `SELECT revision, fetched_at FROM catalog_state`).Scan(&known, &fetched)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	rev, unchanged, cats, err := m.Catalog.Fetch(ctx, known)
	if err != nil {
		if !fetched.IsZero() && time.Since(fetched) > core.CatalogStale {
			m.RT.Logger.Error("the attachment category catalog is stale", "event", "attachment_catalog_stale",
				"fetched_at", fetched, "error", err)
		}
		return err
	}
	return m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		if !unchanged {
			if _, err := tx.Exec(ctx, `DELETE FROM category`); err != nil {
				return err
			}
			for _, c := range cats {
				if _, err := tx.Exec(ctx, `INSERT INTO category (resource_type, name, owner_component, field, max_bytes,
						allowed_types, sensitivity, retention_class, version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
					c.ResourceType, c.Name, c.Owner, c.Field, c.MaxBytes, c.AllowedTypes, c.Sensitivity, c.RetentionClass,
					c.Version); err != nil {
					return err
				}
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO catalog_state (revision, fetched_at) VALUES ($1, now())
			ON CONFLICT (single) DO UPDATE SET revision = excluded.revision, fetched_at = now()`, rev)
		return err
	})
}

// The owner check's answers (AT02).
var (
	// ErrPending is a resource type the copy does not know yet: retriable, no security event.
	ErrPending = errors.New("catalog: the resource type is not in the catalog yet")
	// ErrNotOwner is a caller that is not the resource type's owner, or a category it did not declare.
	ErrNotOwner = errors.New("catalog: not the owner of the resource type, or not its category")
)

// Owner checks that caller owns resourceType and, when category is given, that the category is the resource type's;
// it answers the category.
func Owner(ctx context.Context, tx db.Tx, caller, resourceType, category string) (core.Category, error) {
	var owner string
	err := tx.QueryRow(ctx, `SELECT owner_component FROM category WHERE resource_type = $1 LIMIT 1`, resourceType).Scan(&owner)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return core.Category{}, ErrPending
	case err != nil:
		return core.Category{}, err
	case owner != caller:
		return core.Category{}, ErrNotOwner
	case category == "":
		return core.Category{ResourceType: resourceType, Owner: owner}, nil
	}
	c := core.Category{ResourceType: resourceType, Owner: owner, Name: category}
	err = tx.QueryRow(ctx, `SELECT field, max_bytes, allowed_types, sensitivity, retention_class, version FROM category
		WHERE resource_type = $1 AND name = $2`, resourceType, category).
		Scan(&c.Field, &c.MaxBytes, &c.AllowedTypes, &c.Sensitivity, &c.RetentionClass, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Category{}, ErrNotOwner
	}
	return c, err
}

// Check is the owner check of a system command (AT02): the caller's service identity owns the resource type and the
// category (when given) is its own. A resource type the copy does not know yet answers the retriable
// DEFINITION_PENDING; a caller that is not its owner answers a *Denied error, which rolls the command back: Settle
// then writes its security record in a transaction of its own and answers PERMISSION_DENIED.
func Check(m *core.Base, ctx context.Context, tx db.Tx, resourceType, category string) (string, core.Category, error) {
	caller, ok := grpcx.Caller(ctx)
	if !ok {
		return "", core.Category{}, errs.New(errs.ReasonPermissionDenied, "no calling service")
	}
	c, err := Owner(ctx, tx, caller, resourceType, category)
	switch {
	case errors.Is(err, ErrPending):
		return "", c, errs.New(errs.ReasonDefinitionPending, "the resource type "+resourceType+
			" is not in the attachment category catalog yet")
	case errors.Is(err, ErrNotOwner):
		return "", c, &Denied{Caller: caller, ResourceType: resourceType, Category: category}
	}
	return caller, c, err
}

// Denied is a caller that is not the owner of the resource type, or a category that is not the resource type's.
type Denied struct{ Caller, ResourceType, Category string }

func (d *Denied) Error() string {
	return d.Caller + " is not the owner of " + d.ResourceType + " or " + d.Category + " is not its category"
}

// Settle answers a system command's error: a *Denied one gets its security record (AT02, AT14), after the command's
// own transaction has rolled back, and becomes PERMISSION_DENIED.
func Settle(m *core.Base, ctx context.Context, err error) error {
	var d *Denied
	if !errors.As(err, &d) {
		return err
	}
	if m.RT.Audit != nil {
		if aerr := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.SecurityDenial, Operation: core.OpManage,
				Outcome: audit.Denied, Actor: d.Caller, Reason: "not_the_owner",
				Target: audit.TargetOf(d.ResourceType, d.Category, nil)})
			return err
		}); aerr != nil {
			return aerr
		}
	}
	return errs.New(errs.ReasonPermissionDenied, d.Error())
}
