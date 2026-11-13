// Package reconcile finishes what the record-first steps left and squares the records with the store (docs/design.md
// AT12): every minute the finalizations that did not end and the sessions that expired; every day the orphans, the
// missing objects and their return, the objects of purged and dead versions, the old credentials.
package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	core "github.com/brickKit/infra-attachment/v3/internal/core"
	lifecycle "github.com/brickKit/infra-attachment/v3/internal/lifecycle"
	records "github.com/brickKit/infra-attachment/v3/internal/records"
	uploads "github.com/brickKit/infra-attachment/v3/internal/uploads"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/storage"
	"github.com/brickKit/be-sdk-go/v3/work"
)

// The jobs.
const (
	JobMinute = "reconcile-minute"
	JobDaily  = "reconcile-daily"
)

// Minute redoes the finalizations stuck past FinalizeTimeout (claimed by a conditional update, so two replicas do not
// both redo one) and rejects the uploads whose sessions expired unconfirmed, deleting their staged objects.
func Minute(m *core.Base, ctx context.Context, _ *work.Run) error {
	if m.Store == nil {
		return nil
	}
	type key struct {
		id string
		v  int64
	}
	var stuck []key
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE version SET finalizing_at = now() WHERE (attachment_id, version) IN (
				SELECT attachment_id, version FROM version WHERE state = 'finalizing' AND finalizing_at < now() - $1::interval
				ORDER BY finalizing_at LIMIT 100 FOR UPDATE SKIP LOCKED)
			RETURNING attachment_id::text, version`, core.FinalizeTimeout.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k key
			if err := rows.Scan(&k.id, &k.v); err != nil {
				return err
			}
			stuck = append(stuck, k)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, k := range stuck {
		if err := uploads.Finalize(m, ctx, k.id, k.v); err != nil {
			m.RT.Logger.Warn("a finalization redone did not end", "event", "attachment_finalize_retry",
				"attachment", k.id, "version", k.v, "error", err)
		}
	}
	var expired []key
	err = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE version SET state = 'rejected', reason = 'not confirmed', ended_at = now()
			WHERE (attachment_id, version) IN (SELECT attachment_id, version FROM version WHERE state = 'uploading'
				AND session_expires < now() ORDER BY session_expires LIMIT 500 FOR UPDATE SKIP LOCKED)
			RETURNING attachment_id::text, version`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k key
			if err := rows.Scan(&k.id, &k.v); err != nil {
				rows.Close()
				return err
			}
			expired = append(expired, k)
		}
		rows.Close()
		for _, k := range expired {
			var state string
			if err := tx.QueryRow(ctx, `SELECT state FROM attachment WHERE id = $1::uuid FOR UPDATE`, k.id).Scan(&state); err != nil {
				return err
			}
			if state == core.Uploading { // a new attachment that never arrived (FA29)
				if err := records.SetState(m, ctx, tx, k.id, core.Rejected); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, k := range expired {
		_ = m.Store.Delete(ctx, "staging/"+k.id+"/"+strconv.FormatInt(k.v, 10))
	}
	return nil
}

// Daily squares the records with the store (AT12).
func Daily(m *core.Base, ctx context.Context, _ *work.Run) error {
	if m.Store == nil {
		return nil
	}
	if err := orphans(m, ctx); err != nil {
		return err
	}
	if err := presence(m, ctx); err != nil {
		return err
	}
	var leftovers []string
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		// the purged ones whose objects are still there, and the dead versions past their keeping
		rows, err := tx.Query(ctx, `SELECT DISTINCT a.id::text FROM attachment a JOIN version v ON v.attachment_id = a.id
			WHERE NOT v.object_deleted AND a.state = 'purged'`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			leftovers = append(leftovers, id)
		}
		rows.Close()
		_, err = tx.Exec(ctx, `DELETE FROM credential WHERE expires_at < now() - interval '1 day'`)
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range leftovers {
		lifecycle.DeleteObjects(m, ctx, id)
	}
	return dead(m, ctx)
}

// dead deletes the objects of rejected and quarantined versions kept DeadVersionKept.
func dead(m *core.Base, ctx context.Context) error {
	type key struct {
		id string
		v  int64
	}
	var ks []key
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT attachment_id::text, version FROM version WHERE state IN ('rejected', 'quarantined')
			AND NOT object_deleted AND ended_at < now() - $1::interval LIMIT 1000`, core.DeadVersionKept.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k key
			if err := rows.Scan(&k.id, &k.v); err != nil {
				return err
			}
			ks = append(ks, k)
		}
		return rows.Err()
	})
	for _, k := range ks {
		vs := strconv.FormatInt(k.v, 10)
		if m.Store.Delete(ctx, "objects/"+k.id+"/"+vs) != nil || m.Store.Delete(ctx, "staging/"+k.id+"/"+vs) != nil {
			continue
		}
		_ = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE version SET object_deleted = true WHERE attachment_id = $1::uuid AND version = $2`,
				k.id, k.v)
			return err
		})
	}
	return err
}

// orphans deletes the objects no record points to, older than OrphanWindow (FA12).
func orphans(m *core.Base, ctx context.Context) error {
	for _, prefix := range []string{"objects/", "staging/"} {
		after := ""
		for {
			objs, next, err := m.Store.List(ctx, prefix, after, 500)
			if err != nil {
				return err
			}
			for _, o := range objs {
				if time.Since(o.Modified) < core.OrphanWindow {
					continue
				}
				parts := strings.Split(o.Key, "/") // objects/<attachment>/<version>
				if len(parts) != 3 {
					continue
				}
				v, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil {
					continue
				}
				var known bool
				err = m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
					return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM version WHERE attachment_id::text = $1 AND version = $2
						AND (state IN ('uploading', 'finalizing') OR NOT object_deleted))`, parts[1], v).Scan(&known)
				})
				if err != nil {
					return err
				}
				if !known {
					_ = m.Store.Delete(ctx, o.Key)
				}
			}
			if next == "" {
				break
			}
			after = next
		}
	}
	return nil
}

// presence marks the available attachments whose object is gone missing, and the missing ones whose object came back
// (with the digest recorded) available again (FA13).
func presence(m *core.Base, ctx context.Context) error {
	type row struct {
		id, state string
		v         int64
	}
	var rs []row
	err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, state, current_version FROM attachment WHERE state IN ('available', 'missing')`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.state, &r.v); err != nil {
				return err
			}
			rs = append(rs, r)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, r := range rs {
		_, err := m.Store.Head(ctx, "objects/"+r.id+"/"+strconv.FormatInt(r.v, 10))
		gone := err != nil && isNotFound(err)
		if err != nil && !gone {
			return err
		}
		to := ""
		switch {
		case r.state == core.Available && gone:
			to = core.Missing
			m.RT.Logger.Error("an attachment's object is missing", "event", "attachment_object_missing", "attachment", r.id)
		case r.state == core.Missing && !gone:
			same, err := sameDigest(m, ctx, r.id, r.v)
			if err != nil {
				return err
			}
			if !same {
				m.RT.Logger.Error("a missing attachment's object came back with other content", "event",
					"attachment_object_changed", "attachment", r.id)
				continue
			}
			to = core.Available
		}
		if to == "" {
			continue
		}
		if err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
			var now string
			if err := tx.QueryRow(ctx, `SELECT state FROM attachment WHERE id = $1::uuid FOR UPDATE`, r.id).Scan(&now); err != nil {
				return err
			}
			if now != r.state {
				return nil
			}
			return records.SetState(m, ctx, tx, r.id, to)
		}); err != nil {
			return err
		}
	}
	return nil
}

func isNotFound(err error) bool { return errors.Is(err, storage.ErrNotFound) }

// sameDigest says whether the object at key has the digest recorded for the version (a missing object that came back
// is available again only when it is the same content).
func sameDigest(m *core.Base, ctx context.Context, id string, v int64) (bool, error) {
	var want *string
	if err := m.RT.DB.BackgroundTx(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.QueryRow(ctx, `SELECT sha256 FROM version WHERE attachment_id = $1::uuid AND version = $2`, id, v).Scan(&want)
	}); err != nil || want == nil {
		return false, err
	}
	r, _, err := m.Store.Get(ctx, "objects/"+id+"/"+strconv.FormatInt(v, 10))
	if err != nil {
		return false, err
	}
	defer r.Close()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == *want, nil
}
