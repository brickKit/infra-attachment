// Package uploads is an attachment's content arriving (docs/design.md AT03, AT04, AT05): the owner's upload session,
// the user's confirmation with its credential, and the finalization that copies the staged object to its final key
// and checks the final object itself; the reconciliation redoes a finalization that did not end.
package uploads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	catalog "github.com/brickKit/infra-attachment/v3/internal/catalog"
	core "github.com/brickKit/infra-attachment/v3/internal/core"
	records "github.com/brickKit/infra-attachment/v3/internal/records"

	"github.com/brickKit/be-sdk-go/v3/audit"
	"github.com/brickKit/be-sdk-go/v3/canon"
	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/storage"
	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
	"github.com/jackc/pgx/v5"
)

// Keys of a content version (the glossary): the client uploads to the staging key; the final key is never
// overwritten.
func stagingKey(id string, v int64) string { return "staging/" + id + "/" + strconv.FormatInt(v, 10) }
func finalKey(id string, v int64) string   { return "objects/" + id + "/" + strconv.FormatInt(v, 10) }

// CreateUpload opens an upload session (AT03): a new attachment of the parent with its category snapshot, or a new
// content version of one; the presigned upload address and the confirm credential.
func CreateUpload(m *core.Base, ctx context.Context, in *attachmentv3.CreateUploadRequest) (*attachmentv3.CreateUploadResponse, error) {
	p := in.GetParent()
	switch {
	case p.GetResourceType() == "" || p.GetRecordId() == "":
		return nil, core.Invalid("parent", "the parent's resource type and record are required")
	case in.GetUser() == "":
		return nil, core.Invalid("user", "the user the credential is for is required")
	case strings.TrimSpace(in.GetFileName()) == "" || len(in.GetFileName()) > 255:
		return nil, core.Invalid("file_name", "a file name of 1 to 255 bytes")
	case in.GetSize() <= 0:
		return nil, core.Invalid("size", "the size is positive")
	}
	if m.Store == nil {
		return nil, errs.New(errs.ReasonDependencyUnavailable, "no object store is configured")
	}
	out := &attachmentv3.CreateUploadResponse{}
	var credential string
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var id string
		var v int64
		var limit int64
		var types []string
		if rid := in.GetReplaceAttachmentId(); rid != "" {
			caller, _, err := catalog.Check(m, ctx, tx, p.GetResourceType(), "")
			if err != nil {
				return err
			}
			a, err := records.Load(ctx, tx, caller, p.GetResourceType(), p.GetRecordId(), rid, true)
			if errors.Is(err, records.ErrNotFound) {
				return core.NotFound("attachment")
			} else if err != nil {
				return err
			}
			if in.GetCategory() != "" && in.GetCategory() != a.Category {
				return core.Invalid("category", "a replacement keeps the attachment's category")
			}
			if a.State != core.Available {
				return core.Invalid("replace_attachment_id", "only an available attachment's content is replaced")
			}
			var busy bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM version WHERE attachment_id = $1::uuid
				AND state IN ('uploading', 'finalizing', 'pending_scan'))`, rid).Scan(&busy); err != nil {
				return err
			}
			if busy {
				return core.Invalid("replace_attachment_id", "a replacement of this attachment is in progress")
			}
			if err := tx.QueryRow(ctx, `SELECT max(version) + 1 FROM version WHERE attachment_id = $1::uuid`, rid).Scan(&v); err != nil {
				return err
			}
			id, limit, types = rid, a.MaxBytes, a.AllowedTypes
		} else {
			caller, cat, err := catalog.Check(m, ctx, tx, p.GetResourceType(), in.GetCategory())
			if err != nil {
				return err
			}
			id, v, limit, types = ids.New().String(), 1, min(cat.MaxBytes, core.MaxBytes), cat.AllowedTypes
			if _, err := tx.Exec(ctx, `INSERT INTO attachment (id, owner_component, resource_type, record_id, category,
					category_version, sensitivity, max_bytes, allowed_types, retention_class, state, file_name, uploaded_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'uploading', $11, $12)`, id, caller, p.GetResourceType(),
				p.GetRecordId(), cat.Name, cat.Version, cat.Sensitivity, limit, cat.AllowedTypes, cat.RetentionClass,
				in.GetFileName(), in.GetUser()); err != nil {
				return err
			}
		}
		if in.GetSize() > limit {
			return core.Invalid("size", "the file is larger than its category allows ("+strconv.FormatInt(limit, 10)+" bytes)")
		}
		if !slices.Contains(types, baseType(in.GetContentType())) {
			return core.Invalid("content_type", "the category does not take "+in.GetContentType())
		}
		expires := time.Now().Add(core.UploadTTL)
		if _, err := tx.Exec(ctx, `INSERT INTO version (attachment_id, version, state, staging_key, final_key, declared_size,
				declared_type, session_expires) VALUES ($1, $2, 'uploading', $3, $4, $5, $6, $7)`, id, v, stagingKey(id, v),
			finalKey(id, v), in.GetSize(), baseType(in.GetContentType()), expires); err != nil {
			return err
		}
		value, digest := core.NewSecret()
		if _, err := tx.Exec(ctx, `INSERT INTO credential (digest, operation, principal, attachment_id, version, expires_at)
			VALUES ($1, 'confirm', $2, $3, $4, $5)`, digest, in.GetUser(), id, v, expires); err != nil {
			return err
		}
		url, err := m.Store.PresignPut(stagingKey(id, v), in.GetSize(), baseType(in.GetContentType()), core.UploadTTL)
		if err != nil {
			return err
		}
		if m.RT.Audit != nil { // the issue of a credential is audited as the download's is (AT14)
			a, err := records.ByID(ctx, tx, id, false)
			if err != nil {
				return err
			}
			if _, err := m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: core.OpManage,
				Outcome: audit.Succeeded, Actor: a.Owner, Represented: in.GetUser(),
				Target: audit.TargetOf("infra.attachment", id, map[string]any{"issued": "upload session",
					"content_version": v})}); err != nil {
				return err
			}
		}
		credential = value
		*out = attachmentv3.CreateUploadResponse{AttachmentId: id, Version: v, UploadUrl: url, ExpiresAt: canon.Time(expires)}
		if v == 1 {
			return records.Publish(m, ctx, tx, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.ConfirmCredential = credential
	return out, nil
}

// baseType is a content type without its parameters, lower case.
func baseType(t string) string {
	mt, _, err := mime.ParseMediaType(t)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(t))
	}
	return mt
}

// Confirm is the user's confirmation of an upload with the confirm credential (AT04).
func Confirm(m *core.Base, c *httpx.Context) error {
	ctx, user, err := core.Check(c, core.OpConfirm)
	if err != nil {
		return err
	}
	var body struct {
		Credential string `json:"credential"`
	}
	if err := c.BindJSON(&body); err != nil || body.Credential == "" {
		return core.Invalid("credential", "the confirm credential is required")
	}
	digest := core.Digest(body.Credential)
	// the staged object first: an upload not there yet answers retriable and leaves the credential unused
	var id string
	var v int64
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.QueryRow(ctx, `SELECT attachment_id::text, version FROM credential WHERE digest = $1 AND operation = 'confirm'
			AND principal = $2 AND used_at IS NULL AND expires_at > now()`, digest, user).Scan(&id, &v)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return core.NotFound("upload")
	} else if err != nil {
		return err
	}
	if _, err := m.Store.Head(ctx, stagingKey(id, v)); errors.Is(err, storage.ErrNotFound) {
		return errs.New(errs.ReasonUploadIncomplete, "the upload has not arrived yet")
	} else if err != nil {
		return storeErr(err)
	}
	// the credential used and the version finalizing in one transaction (an atomic conditional update: a replay or a
	// concurrent confirmation finds it used)
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE credential SET used_at = now() WHERE digest = $1 AND operation = 'confirm'
			AND principal = $2 AND used_at IS NULL AND expires_at > now()`, digest, user)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return core.NotFound("upload")
		}
		tag, err = tx.Exec(ctx, `UPDATE version SET state = 'finalizing', finalizing_at = now()
			WHERE attachment_id = $1::uuid AND version = $2 AND state = 'uploading'`, id, v)
		if err != nil || tag.RowsAffected() == 0 {
			if err == nil {
				err = core.NotFound("upload")
			}
			return err
		}
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM attachment WHERE id = $1::uuid FOR UPDATE`, id).Scan(&state); err != nil {
			return err
		}
		if state == core.Uploading {
			return records.SetState(m, ctx, tx, id, core.Finalizing)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := Finalize(m, ctx, id, v); err != nil {
		return err
	}
	var a records.Attachment
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		a, err = records.ByID(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"attachment_id": a.ID, "state": a.State, "version": v, "current_version": a.Version})
}

// Finalize copies a finalizing version's staged object to its final key (only when absent: a retry finds it there)
// and checks the final object itself: its digest and size computed here, its type detected from its content, against
// the attachment's category snapshot; then records the outcome (AT04, AT05). The reconciliation calls it again for a
// version that stayed finalizing.
func Finalize(m *core.Base, ctx context.Context, id string, v int64) error {
	if err := m.Store.Copy(ctx, stagingKey(id, v), finalKey(id, v)); err != nil && !errors.Is(err, storage.ErrExists) {
		if errors.Is(err, storage.ErrNotFound) { // neither the staged object nor a final one: nothing to finalize
			if _, herr := m.Store.Head(ctx, finalKey(id, v)); errors.Is(herr, storage.ErrNotFound) {
				return record(m, ctx, id, v, check{reason: "the uploaded object is gone"})
			}
		} else {
			return storeErr(err)
		}
	}
	r, _, err := m.Store.Get(ctx, finalKey(id, v))
	if err != nil {
		return storeErr(err)
	}
	defer r.Close()
	h := sha256.New()
	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	h.Write(head[:n])
	rest, err := io.Copy(h, r)
	if err != nil {
		return storeErr(err)
	}
	if err := record(m, ctx, id, v, check{size: int64(n) + rest, typ: baseType(http.DetectContentType(head[:n])),
		sha: hex.EncodeToString(h.Sum(nil))}); err != nil {
		return err
	}
	// the staged object has served its purpose (AT12): what the record points to is the final key; a failed delete is
	// the daily reconciliation's (orphans)
	_ = m.Store.Delete(ctx, stagingKey(id, v))
	return nil
}

type check struct {
	size   int64
	typ    string
	sha    string
	reason string // set: the version is rejected for it
}

// record writes a finalization's outcome in one transaction: rejected, or clean (scanning off: unscanned) and then
// current; a new attachment becomes available or rejected with it, a replacement switches its current version.
func record(m *core.Base, ctx context.Context, id string, v int64, c check) error {
	return m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		a, err := records.ByID(ctx, tx, id, true)
		if err != nil {
			return err
		}
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM version WHERE attachment_id = $1::uuid AND version = $2 FOR UPDATE`,
			id, v).Scan(&state); err != nil {
			return err
		}
		if state != core.VFinalizing { // another run recorded it already
			return nil
		}
		if c.reason == "" {
			switch {
			case c.size > a.MaxBytes:
				c.reason = "larger than its category allows"
			case !slices.Contains(a.AllowedTypes, c.typ):
				c.reason = "its content is " + c.typ + ", which its category does not take"
			}
		}
		if c.reason != "" {
			if _, err := tx.Exec(ctx, `UPDATE version SET state = 'rejected', reason = $3, size = $4, detected_type = $5,
				sha256 = $6, ended_at = now() WHERE attachment_id = $1::uuid AND version = $2`, id, v, c.reason,
				nullSize(c.size), nullStr(c.typ), nullStr(c.sha)); err != nil {
				return err
			}
			if a.State == core.Finalizing { // a new attachment: it never became available
				return records.SetState(m, ctx, tx, id, core.Rejected)
			}
			return nil
		}
		// scanning is off in this version (AT05): clean, marked unscanned
		if _, err := tx.Exec(ctx, `UPDATE version SET state = 'clean', size = $3, detected_type = $4, sha256 = $5,
			unscanned = true, scan_result = 'unscanned' WHERE attachment_id = $1::uuid AND version = $2`, id, v, c.size, c.typ,
			c.sha); err != nil {
			return err
		}
		if a.State == core.Finalizing || a.State == core.Uploading {
			if _, err := tx.Exec(ctx, `UPDATE attachment SET current_version = $2 WHERE id = $1::uuid`, id, v); err != nil {
				return err
			}
			return records.SetState(m, ctx, tx, id, core.Available)
		}
		if a.State != core.Available { // detached or erased meanwhile: the new content does not become current
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE version SET state = 'superseded', ended_at = now() WHERE attachment_id = $1::uuid
			AND version = $2`, id, a.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE attachment SET current_version = $2 WHERE id = $1::uuid`, id, v); err != nil {
			return err
		}
		return records.Touch(m, ctx, tx, id)
	})
}

func nullSize(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// storeErr answers an object store failure as the dependency being unavailable (a missing object is the caller's
// to judge).
func storeErr(err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return err
	}
	return errs.New(errs.ReasonDependencyUnavailable, "the object store: "+err.Error())
}
