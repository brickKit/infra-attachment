//go:build integration

package itest

import (
	"context"
	"errors"

	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"

	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/brickKit/be-sdk-go/v3/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// owners: a component that is not the owner of the resource type is refused with a security record; a resource type
// the catalog does not know yet answers DEFINITION_PENDING, retriable, without one (AT02; FA24).
func (fx *fixture) owners() {
	t := fx.t
	_, err := fx.other.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: fx.parent(), Category: "demo.order_contract",
		FileName: "x.pdf", Size: 10, ContentType: "application/pdf", User: sectest.Bob})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "demo/other does not own demo.order")
	_, err = fx.other.List(fx.ctx, &attachmentv3.ListRequest{Parent: fx.parent()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = fx.owner.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: &attachmentv3.Parent{
		ResourceType: "demo.invoice", RecordId: ids.New().String()}, Category: "demo.invoice_file", FileName: "x.pdf",
		Size: 10, ContentType: "application/pdf", User: sectest.Bob})
	assert.Equal(t, codes.Unavailable, status.Code(err), "not in the catalog yet: retriable")
	var denials int
	require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT count(*) FROM "+itSchema+".be_audit_log WHERE category = 'security_denial'"+
		" AND actor->>'id' = 'demo/other'").Scan(&denials))
	assert.Equal(t, 2, denials, "each refusal of a non-owner, and nothing for the pending one")
}

// lifecycle: detaching every attachment of the record, restoring one within the grace period, erasing with a hold
// (kept), releasing it and erasing (purged, objects gone) (AT08, AT10, AT11; FA10, FA11, FA20).
func (fx *fixture) lifecycle() {
	t := fx.t
	list, err := fx.owner.List(fx.ctx, &attachmentv3.ListRequest{Parent: fx.parent()})
	require.NoError(t, err)
	before := len(list.GetAttachments())
	keep := fx.uploaded("demo.order_contract", "keep.pdf", pdf("keep"), "application/pdf")
	inProgress := fx.create("demo.order_contract", "late.pdf", 10, "application/pdf", "")
	det, err := fx.owner.Detach(fx.ctx, &attachmentv3.DetachRequest{Parent: fx.parent(), RetentionStart: "2026-11-13"})
	require.NoError(t, err)
	assert.EqualValues(t, before+2, det.GetDetached(), "every one, the upload in progress too")
	code, _ := fx.confirm(fx.bob, inProgress.GetConfirmCredential())
	assert.Equal(t, 404, code, "the detach used up the credential")
	_, err = fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(), AttachmentId: keep, User: sectest.Bob})
	assert.Error(t, err, "detached: no download")
	r, err := fx.owner.Restore(fx.ctx, &attachmentv3.RestoreRequest{Parent: fx.parent(), AttachmentId: keep})
	require.NoError(t, err)
	assert.Equal(t, "available", r.GetState())

	h, err := fx.owner.Hold(fx.ctx, &attachmentv3.HoldRequest{Parent: fx.parent(), Reason: "litigation"})
	require.NoError(t, err)
	er, err := fx.owner.Erase(fx.ctx, &attachmentv3.EraseRequest{Parent: fx.parent(), AttachmentId: keep})
	require.NoError(t, err)
	assert.Equal(t, []string{keep}, er.GetHeld(), "the parent's hold keeps it")
	assert.Zero(t, er.GetPurged())
	_, err = fx.owner.ReleaseHold(fx.ctx, &attachmentv3.ReleaseHoldRequest{Parent: fx.parent(), HoldId: h.GetHoldId()})
	require.NoError(t, err)
	er, err = fx.owner.Erase(fx.ctx, &attachmentv3.EraseRequest{Parent: fx.parent(), AttachmentId: keep})
	require.NoError(t, err)
	assert.EqualValues(t, 1, er.GetPurged())
	_, err = fx.store.Head(fx.ctx, "objects/"+keep+"/1")
	assert.True(t, errors.Is(err, storage.ErrNotFound), "the object is deleted: %v", err)
	var state string
	require.NoError(t, fx.admin.QueryRow(context.Background(), "SELECT state FROM "+itSchema+".attachment WHERE id = $1", keep).Scan(&state))
	assert.Equal(t, "purged", state, "the record stays")
}

// missing: an available attachment whose object is gone is not reported as absent: the download answers the
// dependency unavailable and the attachment turns missing (AT06, AT12; FA13).
func (fx *fixture) missing() {
	t := fx.t
	fx.record = ids.New().String() // a fresh record: the earlier one's attachments are detached or purged
	id := fx.uploaded("demo.order_contract", "lost.pdf", pdf("lost"), "application/pdf")
	require.NoError(t, fx.store.Delete(fx.ctx, "objects/"+id+"/1"))
	_, err := fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(), AttachmentId: id, User: sectest.Bob})
	assert.Equal(t, codes.Unavailable, status.Code(err), "%v", err)
	list, err := fx.owner.List(fx.ctx, &attachmentv3.ListRequest{Parent: fx.parent()})
	require.NoError(t, err)
	assert.Equal(t, "missing", list.GetAttachments()[0].GetState())
}
