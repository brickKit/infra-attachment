//go:build integration

package itest

import (
	"errors"
	"time"

	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
	lifecycle "github.com/brickKit/infra-attachment/v3/internal/lifecycle"
	reconcile "github.com/brickKit/infra-attachment/v3/internal/reconcile"

	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// purge: the hourly purge takes a detached attachment only when the grace period is over, its retention start is set,
// its class has a minimum retention the deployment gave and it is over, and no hold covers it (AT09, AT11, AT12;
// FA11, FA31, FA35); the staged object is gone once confirmed (AT12); a replacement and a retention start are changes
// of the state event (AT13); the retention start only moves later; a purged attachment cannot be held; the daily
// reconciliation warns of an attachment kept past its class's longest retention (AT09).
func (fx *fixture) purge() {
	t := fx.t
	base := fx.mod.Base()
	fx.record = ids.New().String()
	revision := func(id string) (r int64) {
		require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT revision FROM "+itSchema+".attachment WHERE id = $1", id).Scan(&r))
		return r
	}
	state := func(id string) (s string) {
		require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT state FROM "+itSchema+".attachment WHERE id = $1", id).Scan(&s))
		return s
	}

	// the staged copy is deleted once the final object is written
	noStart := fx.uploaded("demo.order_contract", "no-start.pdf", pdf("no start"), "application/pdf")
	_, err := fx.store.Head(fx.ctx, "staging/"+noStart+"/1")
	assert.True(t, errors.Is(err, storage.ErrNotFound), "the staged object is deleted after the confirmation: %v", err)

	// a replacement moves the event's revision
	before := revision(noStart)
	up := fx.create("", "no-start-2.pdf", len(pdf("two")), "application/pdf", noStart)
	require.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", pdf("two")))
	code, out := fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(t, 200, code, "%v", out)
	assert.Greater(t, revision(noStart), before, "the new current version is a change of the state event")

	// the retention start only moves later, and a move is a change of the state event
	_, err = fx.owner.SetRetentionStart(fx.ctx, &attachmentv3.SetRetentionStartRequest{Parent: fx.parent(),
		AttachmentId: noStart, Date: "2000-01-01"})
	require.NoError(t, err)
	before = revision(noStart)
	_, err = fx.owner.SetRetentionStart(fx.ctx, &attachmentv3.SetRetentionStartRequest{Parent: fx.parent(),
		AttachmentId: noStart, Date: "1999-01-01"})
	require.NoError(t, err)
	var start string
	require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT retention_start::text FROM "+itSchema+".attachment WHERE id = $1",
		noStart).Scan(&start))
	assert.Equal(t, "2000-01-01", start, "an earlier date does not move it back")
	assert.Equal(t, before, revision(noStart), "nothing changed: no event")

	// still attached past the longest retention (3650 days from 2000-01-01): the daily reconciliation warns
	require.NoError(t, reconcile.Daily(base, fx.ctx, nil))
	assert.Contains(t, fx.logs(), "attachment_retention_exceeded", "kept past the class's longest retention")

	// the cases: each on its own record
	detached := func(category, startDate string) string {
		t.Helper()
		fx.record = ids.New().String()
		id := fx.uploaded(category, "case.pdf", pdf("case "+category+startDate), "application/pdf")
		_, err := fx.owner.Detach(fx.ctx, &attachmentv3.DetachRequest{Parent: fx.parent(), RetentionStart: startDate})
		require.NoError(t, err)
		return id
	}
	pending := detached("demo.order_contract", "")                                   // no retention start
	unknown := detached("demo.order_invoice", "2000-01-01")                          // a class the deployment did not give
	young := detached("demo.order_contract", time.Now().UTC().Format(time.DateOnly)) // the minimum is not over
	held := detached("demo.order_contract", "2000-01-01")
	heldParent := fx.parent()
	_, err = fx.owner.Hold(fx.ctx, &attachmentv3.HoldRequest{Parent: heldParent, Reason: "audit"})
	require.NoError(t, err)
	due := detached("demo.order_contract", "2000-01-01")
	inGrace := detached("demo.order_contract", "2000-01-01") // detached just now: the grace period is not over

	_, err = fx.admin.Exec(fx.ctx, "UPDATE "+itSchema+".attachment SET detached_at = now() - interval '31 days' WHERE id = ANY($1)",
		[]string{pending, unknown, young, held, due})
	require.NoError(t, err)
	require.NoError(t, lifecycle.Purge(base, fx.ctx, nil))
	for name, id := range map[string]string{"no retention start": pending, "a class not given": unknown,
		"the minimum not over": young, "a hold on the parent": held, "in the grace period": inGrace} {
		assert.Equal(t, "detached", state(id), "%s: never purged", name)
	}
	assert.Equal(t, "purged", state(due), "every condition holds: purged")
	_, err = fx.store.Head(fx.ctx, "objects/"+due+"/1")
	assert.True(t, errors.Is(err, storage.ErrNotFound), "the purged one's object is deleted: %v", err)
	_, err = fx.store.Head(fx.ctx, "objects/"+held+"/1")
	assert.NoError(t, err, "the held one's object stays")

	// a purged attachment has nothing left to hold
	var record string
	require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT record_id FROM "+itSchema+".attachment WHERE id = $1", due).Scan(&record))
	_, err = fx.owner.Hold(fx.ctx, &attachmentv3.HoldRequest{Parent: &attachmentv3.Parent{ResourceType: "demo.order",
		RecordId: record}, AttachmentId: due, Reason: "too late"})
	assert.Equal(t, codes.NotFound, status.Code(err), "%v", err)
}
