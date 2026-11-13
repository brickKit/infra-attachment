// Package records is the attachment's row as every module reads and writes it: loading it (locked), changing its
// state, and its state event (docs/design.md, the state table; AT13).
package records

import (
	"context"
	"errors"
	"time"

	core "github.com/brickKit/infra-attachment/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/db"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/jackc/pgx/v5"
)

// Attachment is one attachment's row.
type Attachment struct {
	ID              string     `json:"id"`
	Owner           string     `json:"owner_component"`
	ResourceType    string     `json:"resource_type"`
	RecordID        string     `json:"record_id"`
	Category        string     `json:"category"`
	CategoryVersion string     `json:"category_version"`
	Sensitivity     string     `json:"sensitivity"`
	MaxBytes        int64      `json:"-"`
	AllowedTypes    []string   `json:"-"`
	RetentionClass  string     `json:"retention_class"`
	State           string     `json:"state"`
	BeforeDetach    *string    `json:"-"`
	Version         int64      `json:"current_version"`
	FileName        string     `json:"file_name"`
	UploadedBy      string     `json:"-"`
	DetachedAt      *time.Time `json:"-"`
	RetentionStart  *string    `json:"retention_start"`
	Revision        int64      `json:"version"`
	CreatedAt       time.Time  `json:"created_at"`
	// of the current version
	Size      *int64  `json:"size"`
	Type      *string `json:"content_type"`
	Unscanned bool    `json:"unscanned"`
}

const cols = `a.id::text, a.owner_component, a.resource_type, a.record_id, a.category, a.category_version, a.sensitivity,
	a.max_bytes, a.allowed_types, a.retention_class, a.state, a.state_before_detach, a.current_version, a.file_name,
	a.uploaded_by, a.detached_at, a.retention_start::text, a.revision, a.created_at, v.size, v.detected_type,
	coalesce(v.unscanned, false)`

const from = ` FROM attachment a LEFT JOIN version v ON v.attachment_id = a.id AND v.version = a.current_version`

func scan(row pgx.Row) (Attachment, error) {
	var a Attachment
	err := row.Scan(&a.ID, &a.Owner, &a.ResourceType, &a.RecordID, &a.Category, &a.CategoryVersion, &a.Sensitivity,
		&a.MaxBytes, &a.AllowedTypes, &a.RetentionClass, &a.State, &a.BeforeDetach, &a.Version, &a.FileName,
		&a.UploadedBy, &a.DetachedAt, &a.RetentionStart, &a.Revision, &a.CreatedAt, &a.Size, &a.Type, &a.Unscanned)
	return a, err
}

// ErrNotFound is an attachment that is not there, or not of the parent given.
var ErrNotFound = errors.New("records: no such attachment of the parent")

// Load reads an attachment of a parent (the owner's own; lock for a change).
func Load(ctx context.Context, tx db.Tx, owner, resourceType, recordID, id string, lock bool) (Attachment, error) {
	q := `SELECT ` + cols + from + ` WHERE a.id = $1::uuid AND a.owner_component = $2 AND a.resource_type = $3 AND a.record_id = $4`
	if lock {
		q += ` FOR UPDATE OF a`
	}
	a, err := scan(tx.QueryRow(ctx, q, id, owner, resourceType, recordID))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// ByID reads an attachment by its identity alone (the user surface, after a credential named it).
func ByID(ctx context.Context, tx db.Tx, id string, lock bool) (Attachment, error) {
	q := `SELECT ` + cols + from + ` WHERE a.id = $1::uuid`
	if lock {
		q += ` FOR UPDATE OF a`
	}
	a, err := scan(tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// Of lists a parent's attachments, the oldest first, the purged ones left out.
func Of(ctx context.Context, tx db.Tx, owner, resourceType, recordID string, lock bool) ([]Attachment, error) {
	q := `SELECT ` + cols + from + ` WHERE a.owner_component = $1 AND a.resource_type = $2 AND a.record_id = $3
		AND a.state <> 'purged' ORDER BY a.created_at, a.id`
	if lock {
		q += ` FOR UPDATE OF a`
	}
	rows, err := tx.Query(ctx, q, owner, resourceType, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetState writes an attachment's state (the only writer of attachment.state) and publishes its state event in the
// same transaction (AT13).
func SetState(m *core.Base, ctx context.Context, tx db.Tx, id, state string) error {
	if _, err := tx.Exec(ctx, `UPDATE attachment SET state = $2, revision = revision + 1, updated_at = now() WHERE id = $1::uuid`,
		id, state); err != nil {
		return err
	}
	return Publish(m, ctx, tx, id)
}

// Publish emits the attachment's complete state on the component's stream (AT13).
func Publish(m *core.Base, ctx context.Context, tx db.Tx, id string) error {
	if m.RT.Events == nil {
		return nil
	}
	a, err := ByID(ctx, tx, id, false)
	if err != nil {
		return err
	}
	_, err = m.RT.Events.Emit(ctx, tx, core.Stream, events.Event{Type: core.Changed, RecordID: a.ID, Version: a.Revision,
		Projection: events.Full, Data: a})
	return err
}
