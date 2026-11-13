//go:build integration

package itest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"

	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// create opens an upload session as the owner for bob.
func (fx *fixture) create(category, name string, size int, typ, replace string) *attachmentv3.CreateUploadResponse {
	fx.t.Helper()
	up, err := fx.owner.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: fx.parent(), Category: category,
		FileName: name, Size: int64(size), ContentType: typ, User: sectest.Bob, ReplaceAttachmentId: replace})
	require.NoError(fx.t, err)
	return up
}

// confirm confirms as a user.
func (fx *fixture) confirm(c client, credential string) (int, map[string]any) {
	return c.do("POST", "/api/attachment/uploads/confirm", map[string]any{"credential": credential})
}

// uploaded makes an available attachment: session, upload, confirmation.
func (fx *fixture) uploaded(category, name string, body []byte, typ string) string {
	fx.t.Helper()
	up := fx.create(category, name, len(body), typ, "")
	require.Equal(fx.t, 200, fx.put(up.GetUploadUrl(), typ, body))
	code, out := fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(fx.t, 200, code, "%v", out)
	require.Equal(fx.t, "available", out["state"], "%v", out)
	return up.GetAttachmentId()
}

// upload: the owner opens a session, the user uploads through the presigned address and confirms; the attachment is
// available (unscanned: the scan port is off), listed, and its normal download is a presigned address that saves the
// file (AT03, AT04, AT06, AT07; FA05).
func (fx *fixture) upload() {
	t := fx.t
	body := pdf("contract one")
	id := fx.uploaded("demo.order_contract", "合同 1.pdf", body, "application/pdf")
	list, err := fx.owner.List(fx.ctx, &attachmentv3.ListRequest{Parent: fx.parent()})
	require.NoError(t, err)
	require.Len(t, list.GetAttachments(), 1)
	a := list.GetAttachments()[0]
	assert.Equal(t, id, a.GetId())
	assert.Equal(t, "available", a.GetState())
	assert.Equal(t, int64(len(body)), a.GetSize())
	assert.Equal(t, "application/pdf", a.GetContentType())
	assert.True(t, a.GetUnscanned(), "scanning is off: marked unscanned")
	dl, err := fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(), AttachmentId: id, User: sectest.Bob})
	require.NoError(t, err)
	require.NotEmpty(t, dl.GetUrl())
	assert.Empty(t, dl.GetCredential())
	code, got, h := fx.get(dl.GetUrl())
	require.Equal(t, 200, code)
	assert.Equal(t, body, got)
	assert.Contains(t, h.Get("Content-Disposition"), "attachment")
}

// credentials: confirming before the upload arrived answers UPLOAD_INCOMPLETE and leaves the credential usable;
// another user's confirmation, a tampered credential, a second use: 404 (AT04; FA23, FA25).
func (fx *fixture) credentials() {
	t := fx.t
	body := pdf("contract two")
	up := fx.create("demo.order_contract", "c2.pdf", len(body), "application/pdf", "")
	code, out := fx.confirm(fx.bob, up.GetConfirmCredential())
	assert.Equal(t, 409, code, "%v", out)
	assert.Equal(t, "UPLOAD_INCOMPLETE", out["reason"])
	require.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", body))
	code, _ = fx.confirm(fx.carol, up.GetConfirmCredential())
	assert.Equal(t, 404, code, "carol may not use bob's credential")
	code, _ = fx.confirm(fx.bob, up.GetConfirmCredential()+"x")
	assert.Equal(t, 404, code, "tampered")
	code, out = fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(t, 200, code, "%v", out)
	code, _ = fx.confirm(fx.bob, up.GetConfirmCredential())
	assert.Equal(t, 404, code, "used once")
	code, _ = fx.confirm(fx.dave, up.GetConfirmCredential())
	assert.Contains(t, []int{403, 404}, code, "no grant")
}

// content: the content checked is the final object's: a file of another type than declared is rejected by what the
// server detects; uploading again through the still valid address after the confirmation changes nothing; a size
// beyond the category is refused at the session (AT03, AT04; FA02, FA03, FA21).
func (fx *fixture) content() {
	t := fx.t
	html := []byte("<html><body><script>alert(1)</script></body></html>")
	up := fx.create("demo.order_contract", "fake.pdf", len(html), "application/pdf", "")
	require.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", html))
	code, out := fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(t, 200, code, "%v", out)
	assert.Equal(t, "rejected", out["state"], "detected text/html, not a PDF")

	body := pdf("the real one")
	up = fx.create("demo.order_contract", "real.pdf", len(body), "application/pdf", "")
	require.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", body))
	code, out = fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(t, 200, code, "%v", out)
	evil := pdf("the swapped one")
	evil = append(evil[:len(body)-1], '\n')[:len(body)] // same length: the address still takes it
	assert.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", evil), "the staging key still takes it")
	dl, err := fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(),
		AttachmentId: up.GetAttachmentId(), User: sectest.Bob})
	require.NoError(t, err)
	_, got, _ := fx.get(dl.GetUrl())
	assert.Equal(t, body, got, "the final object is what was confirmed")

	_, err = fx.owner.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: fx.parent(), Category: "demo.order_contract",
		FileName: "big.pdf", Size: 2 << 20, ContentType: "application/pdf", User: sectest.Bob})
	assert.Error(t, err, "beyond the category's limit")
	_, err = fx.owner.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: fx.parent(), Category: "demo.order_contract",
		FileName: "x.svg", Size: 10, ContentType: "image/svg+xml", User: sectest.Bob})
	assert.Error(t, err, "a type the category does not take")
}

// strict: a strict category's download is a one-time credential bob presents here; the bytes come with headers that
// save them; a second use, or carol with bob's credential: 404 (AT06; FA09, FA22, FA25).
func (fx *fixture) strict() {
	t := fx.t
	body := pdf("an invoice scan")
	id := fx.uploaded("demo.order_invoice", "发票.pdf", body, "application/pdf")
	dl, err := fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(), AttachmentId: id, User: sectest.Bob})
	require.NoError(t, err)
	assert.Empty(t, dl.GetUrl(), "no presigned address for a strict category")
	require.NotEmpty(t, dl.GetCredential())
	code, _, _ := fx.carol.raw("POST", "/api/attachment/downloads", map[string]any{"credential": dl.GetCredential()})
	assert.Equal(t, 404, code, "bob's credential")
	code, got, h := fx.bob.raw("POST", "/api/attachment/downloads", map[string]any{"credential": dl.GetCredential()})
	require.Equal(t, 200, code, string(got))
	assert.Equal(t, body, got)
	assert.Contains(t, h.Get("Content-Disposition"), "attachment")
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Contains(t, h.Get("Content-Security-Policy"), "sandbox")
	code, _, _ = fx.bob.raw("POST", "/api/attachment/downloads", map[string]any{"credential": dl.GetCredential()})
	assert.Equal(t, 404, code, "used once")
}

// replace: a new content version goes through the same confirmation; the downloads give the old one until the new is
// clean, then the new one; a second replacement while one is in progress is refused (AT03; FA06, FA30).
func (fx *fixture) replace() {
	t := fx.t
	first := pdf("version one")
	id := fx.uploaded("demo.order_contract", "v.pdf", first, "application/pdf")
	second := pdf("version two, longer")
	up := fx.create("demo.order_contract", "v.pdf", len(second), "application/pdf", id)
	assert.Equal(t, int64(2), up.GetVersion())
	_, err := fx.owner.CreateUpload(fx.ctx, &attachmentv3.CreateUploadRequest{Parent: fx.parent(), FileName: "v.pdf",
		Size: 5, ContentType: "application/pdf", User: sectest.Bob, ReplaceAttachmentId: id})
	assert.Error(t, err, "one replacement at a time")
	download := func() []byte {
		dl, err := fx.owner.IssueDownload(fx.ctx, &attachmentv3.IssueDownloadRequest{Parent: fx.parent(), AttachmentId: id, User: sectest.Bob})
		require.NoError(t, err)
		_, got, _ := fx.get(dl.GetUrl())
		return got
	}
	assert.Equal(t, first, download(), "the current version while the new one is not confirmed")
	require.Equal(t, 200, fx.put(up.GetUploadUrl(), "application/pdf", second))
	code, out := fx.confirm(fx.bob, up.GetConfirmCredential())
	require.Equal(t, 200, code, "%v", out)
	assert.EqualValues(t, 2, out["current_version"])
	assert.Equal(t, second, download())
	var state string
	require.NoError(t, fx.admin.QueryRow(fx.ctx, "SELECT state FROM "+itSchema+".version WHERE attachment_id = $1 AND version = 1",
		id).Scan(&state))
	assert.Equal(t, "superseded", state, "kept, no longer current")
}

// raw is a call whose answer is bytes.
func (c client) raw(method, path string, body any) (int, []byte, http.Header) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, itBase+path, r)
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}
