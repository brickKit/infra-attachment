// Package downloads is an available attachment going to a user (docs/design.md AT06): the owner's request for a
// presigned address (a normal category) or a one-time credential (a strict one), and the user's strict download
// through this component with headers that keep the content from running.
package downloads

import (
	"context"
	"errors"
	"strconv"
	"time"

	catalog "github.com/brickKit/infra-attachment/v3/internal/catalog"
	core "github.com/brickKit/infra-attachment/v3/internal/core"
	records "github.com/brickKit/infra-attachment/v3/internal/records"

	"github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/storage"
	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
	"github.com/jackc/pgx/v5"
)

func finalKey(id string, v int64) string { return "objects/" + id + "/" + strconv.FormatInt(v, 10) }

// shown are the types a strict download is answered with as they are; anything else is a plain stream of bytes.
var shown = map[string]bool{"application/pdf": true, "image/png": true, "image/jpeg": true, "image/gif": true,
	"image/webp": true}

// IssueDownload gives the user the attachment's current version (AT06).
func IssueDownload(m *core.Base, ctx context.Context, in *attachmentv3.IssueDownloadRequest) (*attachmentv3.IssueDownloadResponse, error) {
	p := in.GetParent()
	if in.GetUser() == "" || in.GetAttachmentId() == "" {
		return nil, core.Invalid("user", "the attachment and the user are required")
	}
	if m.Store == nil {
		return nil, errs.New(errs.ReasonDependencyUnavailable, "no object store is configured")
	}
	var a records.Attachment
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, p.GetResourceType(), "")
		if err != nil {
			return err
		}
		a, err = records.Load(ctx, tx, caller, p.GetResourceType(), p.GetRecordId(), in.GetAttachmentId(), false)
		if errors.Is(err, records.ErrNotFound) {
			return core.NotFound("attachment")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if a.State != core.Available {
		return nil, core.Invalid("attachment_id", "the attachment is "+a.State+", not available")
	}
	// the object first: one gone is not "no such attachment" (FA13)
	if _, err := m.Store.Head(ctx, finalKey(a.ID, a.Version)); errors.Is(err, storage.ErrNotFound) {
		if err := missing(m, ctx, a.ID); err != nil {
			return nil, err
		}
		return nil, errs.New(errs.ReasonDependencyUnavailable, "the attachment's object is missing")
	} else if err != nil {
		return nil, errs.New(errs.ReasonDependencyUnavailable, "the object store: "+err.Error())
	}
	if a.Sensitivity == core.SensitivityNormal {
		url, err := m.Store.PresignGet(finalKey(a.ID, a.Version), core.DownloadTTL, a.FileName, "")
		if err != nil {
			return nil, err
		}
		if err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
			return issued(m, ctx, tx, a, in.GetUser(), "presigned")
		}); err != nil {
			return nil, err
		}
		return &attachmentv3.IssueDownloadResponse{Url: url, ExpiresAt: canon.Time(time.Now().Add(core.DownloadTTL))}, nil
	}
	value, digest := core.NewSecret()
	expires := time.Now().Add(core.StrictTTL)
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO credential (digest, operation, principal, attachment_id, version, expires_at)
			VALUES ($1, 'download', $2, $3, $4, $5)`, digest, in.GetUser(), a.ID, a.Version, expires); err != nil {
			return err
		}
		return issued(m, ctx, tx, a, in.GetUser(), "one-time credential")
	})
	if err != nil {
		return nil, err
	}
	return &attachmentv3.IssueDownloadResponse{Credential: value, ExpiresAt: canon.Time(expires)}, nil
}

// issued records the issue of a download in the audit (AT14).
func issued(m *core.Base, ctx context.Context, tx db.Tx, a records.Attachment, user, how string) error {
	if m.RT.Audit == nil {
		return nil
	}
	_, err := m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: core.OpManage,
		Outcome: audit.Succeeded, Actor: a.Owner, Represented: user,
		Target: audit.TargetOf("infra.attachment", a.ID, nil), Changes: map[string]any{"issued": how, "content_version": a.Version}})
	return err
}

// missing marks an available attachment whose object is gone (AT12, FA13): not available, an alert.
func missing(m *core.Base, ctx context.Context, id string) error {
	m.RT.Logger.Error("an attachment's object is missing", "event", "attachment_object_missing", "attachment", id)
	return m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM attachment WHERE id = $1::uuid FOR UPDATE`, id).Scan(&state); err != nil {
			return err
		}
		if state != core.Available {
			return nil
		}
		return records.SetState(m, ctx, tx, id, core.Missing)
	})
}

// Download is the user's strict download with a one-time credential (AT06): used atomically, bound to the user, for
// the version it was issued for; the bytes stream through with headers that save and never run them.
func Download(m *core.Base, c *httpx.Context) error {
	ctx, user, err := core.Check(c, core.OpDownload)
	if err != nil {
		return err
	}
	var body struct {
		Credential string `json:"credential"`
	}
	if err := c.BindJSON(&body); err != nil || body.Credential == "" {
		return core.Invalid("credential", "the download credential is required")
	}
	var a records.Attachment
	var v int64
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `UPDATE credential SET used_at = now() WHERE digest = $1 AND operation = 'download'
			AND principal = $2 AND used_at IS NULL AND expires_at > now() RETURNING attachment_id::text, version`,
			core.Digest(body.Credential), user).Scan(&id, &v)
		if errors.Is(err, pgx.ErrNoRows) {
			return core.NotFound("download")
		} else if err != nil {
			return err
		}
		a, err = records.ByID(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if a.State != core.Available || a.Version != v {
			return core.NotFound("download")
		}
		if m.RT.Audit != nil {
			_, err = m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.SensitiveRead, Operation: core.OpDownload,
				Outcome: audit.Succeeded, Actor: user, Target: audit.TargetOf("infra.attachment", a.ID, nil),
				Changes: map[string]any{"content_version": v}})
		}
		return err
	})
	if err != nil {
		return err
	}
	r, meta, err := m.Store.Get(ctx, finalKey(a.ID, v))
	if errors.Is(err, storage.ErrNotFound) {
		if err := missing(m, ctx, a.ID); err != nil {
			return err
		}
		return errs.New(errs.ReasonDependencyUnavailable, "the attachment's object is missing")
	} else if err != nil {
		return errs.New(errs.ReasonDependencyUnavailable, "the object store: "+err.Error())
	}
	defer r.Close()
	typ := "application/octet-stream"
	if a.Type != nil && shown[*a.Type] {
		typ = *a.Type
	}
	return c.Download(typ, a.FileName, meta.Size, r) // saved, never run (the SDK's headers)
}
