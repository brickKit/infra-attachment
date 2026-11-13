// Package lifecycle is what the owner commands of its attachments after they arrive (docs/design.md AT07-AT11): the
// list, detaching and restoring, the retention start, erasure, holds, and the purge once the grace period and the
// minimum retention are over. Each change is the record first and the objects after: the reconciliation finishes what
// a crash left (AT12).
package lifecycle

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
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/work"
	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
)

// JobPurge is the purge of the detached attachments past their grace and retention, and of the rejected ones.
const JobPurge = "purge-attachments"

// List answers the parent's attachments (AT07).
func List(m *core.Base, ctx context.Context, in *attachmentv3.ListRequest) (*attachmentv3.ListResponse, error) {
	p := in.GetParent()
	out := &attachmentv3.ListResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, p.GetResourceType(), "")
		if err != nil {
			return err
		}
		as, err := records.Of(ctx, tx, caller, p.GetResourceType(), p.GetRecordId(), false)
		for _, a := range as {
			e := &attachmentv3.Attachment{Id: a.ID, Category: a.Category, State: a.State, FileName: a.FileName,
				Version: a.Version, Unscanned: a.Unscanned, CreatedAt: canon.Time(a.CreatedAt), Sensitivity: a.Sensitivity}
			if a.Size != nil {
				e.Size = *a.Size
			}
			if a.Type != nil {
				e.ContentType = *a.Type
			}
			out.Attachments = append(out.Attachments, e)
		}
		return err
	})
	return out, err
}

// targets are the attachments a command names: one, or every one of the parent (locked).
func targets(ctx context.Context, tx db.Tx, owner string, p *attachmentv3.Parent, id string) ([]records.Attachment, error) {
	if id == "" {
		return records.Of(ctx, tx, owner, p.GetResourceType(), p.GetRecordId(), true)
	}
	a, err := records.Load(ctx, tx, owner, p.GetResourceType(), p.GetRecordId(), id, true)
	if errors.Is(err, records.ErrNotFound) {
		return nil, core.NotFound("attachment")
	}
	return []records.Attachment{a}, err
}

func date(s string) (*string, error) {
	if s == "" {
		return nil, nil
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return nil, core.Invalid("date", "a date is YYYY-MM-DD")
	}
	return &s, nil
}

// Detach takes attachments off their parent (AT08): any state but purged; the uploads in progress are rejected and
// their credentials used up; a retention start may come with it (AT09).
func Detach(m *core.Base, ctx context.Context, in *attachmentv3.DetachRequest) (*attachmentv3.DetachResponse, error) {
	start, err := date(in.GetRetentionStart())
	if err != nil {
		return nil, err
	}
	out := &attachmentv3.DetachResponse{}
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, in.GetParent().GetResourceType(), "")
		if err != nil {
			return err
		}
		as, err := targets(ctx, tx, caller, in.GetParent(), in.GetAttachmentId())
		if err != nil {
			return err
		}
		for _, a := range as {
			if start != nil {
				if _, err := tx.Exec(ctx, `UPDATE attachment SET retention_start = $2::date WHERE id = $1::uuid`, a.ID, *start); err != nil {
					return err
				}
			}
			if a.State == core.Detached || a.State == core.Purged {
				continue
			}
			if err := endUploads(ctx, tx, a.ID, "detached"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE attachment SET state_before_detach = state, detached_at = now()
				WHERE id = $1::uuid`, a.ID); err != nil {
				return err
			}
			if err := records.SetState(m, ctx, tx, a.ID, core.Detached); err != nil {
				return err
			}
			if err := audited(m, ctx, tx, caller, a.ID, "detached"); err != nil {
				return err
			}
			out.Detached++
		}
		return nil
	})
	return out, err
}

// endUploads rejects an attachment's versions in progress and uses up their credentials.
func endUploads(ctx context.Context, tx db.Tx, id, reason string) error {
	if _, err := tx.Exec(ctx, `UPDATE version SET state = 'rejected', reason = $2, ended_at = now() WHERE attachment_id = $1::uuid
		AND state IN ('uploading', 'finalizing', 'pending_scan')`, id, reason); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE credential SET used_at = now() WHERE attachment_id = $1::uuid AND used_at IS NULL`, id)
	return err
}

// Restore puts a detached attachment back within its grace (AT08): to the state it had; an upload that was in
// progress was rejected by the detach, so the attachment comes back rejected.
func Restore(m *core.Base, ctx context.Context, in *attachmentv3.RestoreRequest) (*attachmentv3.RestoreResponse, error) {
	out := &attachmentv3.RestoreResponse{}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, in.GetParent().GetResourceType(), "")
		if err != nil {
			return err
		}
		as, err := targets(ctx, tx, caller, in.GetParent(), in.GetAttachmentId())
		if err != nil {
			return err
		}
		if in.GetAttachmentId() == "" {
			return core.Invalid("attachment_id", "the attachment to restore is required")
		}
		a := as[0]
		switch {
		case a.State != core.Detached:
			return core.Invalid("attachment_id", "the attachment is "+a.State+", not detached")
		case a.DetachedAt != nil && time.Since(*a.DetachedAt) > core.Grace:
			return core.Invalid("attachment_id", "the grace period is over")
		}
		to := core.Rejected
		if a.BeforeDetach != nil && (*a.BeforeDetach == core.Available || *a.BeforeDetach == core.Quarantined ||
			*a.BeforeDetach == core.Missing) {
			to = *a.BeforeDetach
		}
		if _, err := tx.Exec(ctx, `UPDATE attachment SET detached_at = NULL, state_before_detach = NULL WHERE id = $1::uuid`,
			a.ID); err != nil {
			return err
		}
		out.State = to
		return records.SetState(m, ctx, tx, a.ID, to)
	})
	return out, err
}

// SetRetentionStart sets the date the retention counts from (AT09): one attachment or every one of the parent.
func SetRetentionStart(m *core.Base, ctx context.Context, in *attachmentv3.SetRetentionStartRequest) (*attachmentv3.SetRetentionStartResponse, error) {
	start, err := date(in.GetDate())
	if err != nil || start == nil {
		return nil, core.Invalid("date", "a date is YYYY-MM-DD")
	}
	out := &attachmentv3.SetRetentionStartResponse{}
	err = m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, in.GetParent().GetResourceType(), "")
		if err != nil {
			return err
		}
		as, err := targets(ctx, tx, caller, in.GetParent(), in.GetAttachmentId())
		for _, a := range as {
			if _, err := tx.Exec(ctx, `UPDATE attachment SET retention_start = $2::date WHERE id = $1::uuid`, a.ID, *start); err != nil {
				return err
			}
			out.Set++
		}
		return err
	})
	return out, err
}

// held says whether a live hold covers the attachment: one on it, or one on its parent (AT11).
func held(ctx context.Context, tx db.Tx, a records.Attachment) (bool, error) {
	var h bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hold WHERE released_at IS NULL AND owner_component = $1
		AND resource_type = $2 AND record_id = $3 AND (attachment_id IS NULL OR attachment_id = $4::uuid))`,
		a.Owner, a.ResourceType, a.RecordID, a.ID).Scan(&h)
	return h, err
}

// Erase purges at once (AT10): one attachment, or the parent's of the categories given; one under hold is kept and
// reported, with an alert.
func Erase(m *core.Base, ctx context.Context, in *attachmentv3.EraseRequest) (*attachmentv3.EraseResponse, error) {
	if in.GetAttachmentId() == "" && len(in.GetCategories()) == 0 {
		return nil, core.Invalid("categories", "an attachment or the categories to erase are required")
	}
	out := &attachmentv3.EraseResponse{}
	var purged []string
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		caller, _, err := catalog.Check(m, ctx, tx, in.GetParent().GetResourceType(), "")
		if err != nil {
			return err
		}
		as, err := targets(ctx, tx, caller, in.GetParent(), in.GetAttachmentId())
		if err != nil {
			return err
		}
		for _, a := range as {
			if a.State == core.Purged || (in.GetAttachmentId() == "" && !contains(in.GetCategories(), a.Category)) {
				continue
			}
			h, err := held(ctx, tx, a)
			if err != nil {
				return err
			}
			if h {
				m.RT.Logger.Warn("an erasure met a hold: the attachment is kept", "event", "attachment_erase_held",
					"attachment", a.ID)
				out.Held = append(out.Held, a.ID)
				continue
			}
			if err := purge(m, ctx, tx, caller, a.ID, "erased"); err != nil {
				return err
			}
			purged = append(purged, a.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, id := range purged {
		DeleteObjects(m, ctx, id)
	}
	out.Purged = int32(len(purged))
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// purge marks an attachment purged (its uploads in progress ended); the objects go after the commit.
func purge(m *core.Base, ctx context.Context, tx db.Tx, actor, id, why string) error {
	if err := endUploads(ctx, tx, id, why); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE attachment SET purged_at = now() WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	if err := records.SetState(m, ctx, tx, id, core.Purged); err != nil {
		return err
	}
	return audited(m, ctx, tx, actor, id, why)
}

// DeleteObjects deletes every version's objects of a purged attachment (best effort: the reconciliation deletes what
// is left, AT12).
func DeleteObjects(m *core.Base, ctx context.Context, id string) {
	if m.Store == nil {
		return
	}
	var versions []int64
	_ = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT version FROM version WHERE attachment_id = $1::uuid AND NOT object_deleted`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				return err
			}
			versions = append(versions, v)
		}
		return rows.Err()
	})
	for _, v := range versions {
		vs := strconv.FormatInt(v, 10)
		if m.Store.Delete(ctx, "objects/"+id+"/"+vs) != nil || m.Store.Delete(ctx, "staging/"+id+"/"+vs) != nil {
			continue
		}
		_ = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE version SET object_deleted = true WHERE attachment_id = $1::uuid AND version = $2`, id, v)
			return err
		})
	}
}

// audited records a lifecycle command in the audit (AT14).
func audited(m *core.Base, ctx context.Context, tx db.Tx, actor, id, what string) error {
	if m.RT.Audit == nil {
		return nil
	}
	_, err := m.RT.Audit.Write(ctx, tx, audit.Record{Category: audit.BusinessChange, Operation: core.OpManage,
		Outcome: audit.Succeeded, Actor: actor, Target: audit.TargetOf("infra.attachment", id, map[string]any{"change": what})})
	return err
}

// Hold places a hold on one attachment or on every one of the parent, those to come included (AT11).
func Hold(m *core.Base, ctx context.Context, in *attachmentv3.HoldRequest) (*attachmentv3.HoldResponse, error) {
	if in.GetReason() == "" {
		return nil, core.Invalid("reason", "a hold says why")
	}
	out := &attachmentv3.HoldResponse{HoldId: ids.New().String()}
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		p := in.GetParent()
		caller, _, err := catalog.Check(m, ctx, tx, p.GetResourceType(), "")
		if err != nil {
			return err
		}
		var att *string
		if id := in.GetAttachmentId(); id != "" {
			if _, err := targets(ctx, tx, caller, p, id); err != nil {
				return err
			}
			att = &id
		}
		_, err = tx.Exec(ctx, `INSERT INTO hold (id, owner_component, resource_type, record_id, attachment_id, reason)
			VALUES ($1, $2, $3, $4, $5::uuid, $6)`, out.HoldId, caller, p.GetResourceType(), p.GetRecordId(), att, in.GetReason())
		return err
	})
	return out, err
}

// ReleaseHold releases a hold of the parent.
func ReleaseHold(m *core.Base, ctx context.Context, in *attachmentv3.ReleaseHoldRequest) (*attachmentv3.ReleaseHoldResponse, error) {
	err := m.RT.DB.Tx(ctx, func(ctx context.Context, tx db.Tx) error {
		p := in.GetParent()
		caller, _, err := catalog.Check(m, ctx, tx, p.GetResourceType(), "")
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE hold SET released_at = now() WHERE id = $1::uuid AND owner_component = $2
			AND resource_type = $3 AND record_id = $4 AND released_at IS NULL`, in.GetHoldId(), caller, p.GetResourceType(),
			p.GetRecordId())
		if err == nil && tag.RowsAffected() == 0 {
			return core.NotFound("hold")
		}
		return err
	})
	return &attachmentv3.ReleaseHoldResponse{}, err
}

// Purge purges what is due (AT09, AT12): the detached attachments past the grace period whose retention start is set,
// whose class's minimum retention is over (a class the deployment did not give is to be confirmed: never) and that no
// hold covers; and the rejected attachments whose sessions are over. It alerts the retention starts pending for over
// a year.
func Purge(m *core.Base, ctx context.Context, _ *work.Run) error {
	var due []string
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.id::text, a.retention_class, a.retention_start::text, a.state FROM attachment a
			WHERE (a.state = 'detached' AND a.detached_at < now() - $1::interval)
			   OR (a.state = 'rejected' AND NOT EXISTS (SELECT 1 FROM version v WHERE v.attachment_id = a.id
			       AND v.session_expires > now()))
			ORDER BY a.id LIMIT 500 FOR UPDATE SKIP LOCKED`, core.Grace.String())
		if err != nil {
			return err
		}
		type cand struct {
			id, class, state string
			start            *string
		}
		var cands []cand
		for rows.Next() {
			var c cand
			if err := rows.Scan(&c.id, &c.class, &c.start, &c.state); err != nil {
				rows.Close()
				return err
			}
			cands = append(cands, c)
		}
		rows.Close()
		for _, c := range cands {
			a, err := records.ByID(ctx, tx, c.id, false)
			if err != nil {
				return err
			}
			if c.state == core.Detached {
				r, known := m.Retention[c.class]
				switch {
				case c.start == nil:
					if time.Since(a.CreatedAt) > 365*24*time.Hour {
						m.RT.Logger.Warn("an attachment's retention start is pending for over a year", "event",
							"attachment_retention_pending", "attachment", c.id)
					}
					continue
				case !known: // to be confirmed: never purged (protocol 11)
					continue
				}
				start, _ := time.Parse(time.DateOnly, *c.start)
				if time.Now().Before(start.AddDate(0, 0, r.MinDays)) {
					continue
				}
				if h, err := held(ctx, tx, a); err != nil || h {
					if err != nil {
						return err
					}
					continue
				}
			}
			if err := purge(m, ctx, tx, core.ID, c.id, "purged after "+c.state); err != nil {
				return err
			}
			due = append(due, c.id)
		}
		return nil
	})
	for _, id := range due {
		DeleteObjects(m, ctx, id)
	}
	return err
}
