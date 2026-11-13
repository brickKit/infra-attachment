package attachment

import (
	"context"

	catalog "github.com/brickKit/infra-attachment/v3/internal/catalog"
	downloads "github.com/brickKit/infra-attachment/v3/internal/downloads"
	lifecycle "github.com/brickKit/infra-attachment/v3/internal/lifecycle"
	uploads "github.com/brickKit/infra-attachment/v3/internal/uploads"

	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
)

// The system surface's methods, each handed to its module.

func (m *Module) CreateUpload(ctx context.Context, in *attachmentv3.CreateUploadRequest) (*attachmentv3.CreateUploadResponse, error) {
	out, err := uploads.CreateUpload(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) IssueDownload(ctx context.Context, in *attachmentv3.IssueDownloadRequest) (*attachmentv3.IssueDownloadResponse, error) {
	out, err := downloads.IssueDownload(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) List(ctx context.Context, in *attachmentv3.ListRequest) (*attachmentv3.ListResponse, error) {
	out, err := lifecycle.List(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) Detach(ctx context.Context, in *attachmentv3.DetachRequest) (*attachmentv3.DetachResponse, error) {
	out, err := lifecycle.Detach(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) Restore(ctx context.Context, in *attachmentv3.RestoreRequest) (*attachmentv3.RestoreResponse, error) {
	out, err := lifecycle.Restore(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) SetRetentionStart(ctx context.Context, in *attachmentv3.SetRetentionStartRequest) (*attachmentv3.SetRetentionStartResponse, error) {
	out, err := lifecycle.SetRetentionStart(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) Erase(ctx context.Context, in *attachmentv3.EraseRequest) (*attachmentv3.EraseResponse, error) {
	out, err := lifecycle.Erase(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) Hold(ctx context.Context, in *attachmentv3.HoldRequest) (*attachmentv3.HoldResponse, error) {
	out, err := lifecycle.Hold(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}

func (m *Module) ReleaseHold(ctx context.Context, in *attachmentv3.ReleaseHoldRequest) (*attachmentv3.ReleaseHoldResponse, error) {
	out, err := lifecycle.ReleaseHold(m.b, ctx, in)
	return out, catalog.Settle(m.b, ctx, err)
}
