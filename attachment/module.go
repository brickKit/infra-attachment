package attachment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	catalog "github.com/brickKit/infra-attachment/v3/internal/catalog"
	core "github.com/brickKit/infra-attachment/v3/internal/core"
	downloads "github.com/brickKit/infra-attachment/v3/internal/downloads"
	lifecycle "github.com/brickKit/infra-attachment/v3/internal/lifecycle"
	reconcile "github.com/brickKit/infra-attachment/v3/internal/reconcile"
	uploads "github.com/brickKit/infra-attachment/v3/internal/uploads"

	"github.com/brickKit/be-sdk-go/v3/grpcx"
	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/storage"
	"github.com/brickKit/be-sdk-go/v3/work"
	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
)

// Module is the attachment component as the launcher and the shells run it: it only assembles the business modules
// under internal/ (their map is internal/AGENTS.md).
type Module struct {
	attachmentv3.UnimplementedAttachmentServiceServer
	b *core.Base
}

// New builds the module from its runtime: the object store (required), the catalog's source (the authorization
// member), the retention classes and the scan port (a malformed or unknown value stops the member).
func New(rt *runtime.Runtime) (*Module, error) {
	if scan := rt.Config.String(core.KeyScan); scan != core.ScanOff {
		return nil, fmt.Errorf("%s: %q: this version has the scan port off only", core.KeyScan, scan)
	}
	ret := map[string]core.Retention{}
	if raw := rt.Config.String(core.KeyRetention); raw != "" {
		if err := json.Unmarshal([]byte(raw), &ret); err != nil {
			return nil, fmt.Errorf("%s: %w", core.KeyRetention, err)
		}
	}
	store, err := storage.From(rt.Config, rt.Logger)
	if err != nil {
		return nil, err
	}
	b := &core.Base{RT: rt, Store: store, Retention: ret, Scan: core.ScanOff}
	if conn, ok := rt.GRPCAt("infra/authz", rt.Config.String("AUTHZ_GRPC_URL")); ok {
		b.Catalog = catalog.GRPC{Conn: conn}
	}
	return &Module{b: b}, nil
}

// UseCatalog replaces the catalog's source (the integration tests: the authorization member's catalog needs its
// definitions).
func (m *Module) UseCatalog(c core.CatalogSource) { m.b.Catalog = c }

// Routes registers the user surface (contract/openapi.yaml): the confirmation and the strict download, each with the
// credential an owner had issued.
func (m *Module) Routes(r *httpx.Routes) error {
	h := func(f func(*core.Base, *httpx.Context) error) httpx.Handler {
		return func(c *httpx.Context) error { return f(m.b, c) }
	}
	r.Add(httpx.Route{Method: "POST", Path: "/api/attachment/uploads/confirm", Operation: core.OpConfirm, Handle: h(uploads.Confirm)})
	r.Add(httpx.Route{Method: "POST", Path: "/api/attachment/downloads", Operation: core.OpDownload, Handle: h(downloads.Download)})
	return nil
}

// Services registers the system surface (contract/proto): any component may call; the owner check is per command
// (AT02).
func (m *Module) Services(s *grpcx.Services) error {
	any := grpcx.Method{Operation: core.OpManage, Callers: []string{grpcx.AnyComponent}}
	methods := map[string]grpcx.Method{}
	for _, name := range []string{"CreateUpload", "IssueDownload", "List", "Detach", "Restore", "SetRetentionStart", "Erase",
		"Hold", "ReleaseHold"} {
		methods[name] = any
	}
	s.Register(&attachmentv3.AttachmentService_ServiceDesc, m, methods)
	return nil
}

// Jobs declares the background work (protocol 01): the catalog every minute, the minute's and the day's
// reconciliation, the purge every hour.
func (m *Module) Jobs(j *work.Jobs) error {
	job := func(name string, every time.Duration, f func(*core.Base, context.Context, *work.Run) error) {
		j.Add(work.Job{Name: name, Kind: work.Singleton, Effect: work.Local, Interval: every, MaxAttempts: work.Unlimited,
			Handle: func(ctx context.Context, r *work.Run) error { return f(m.b, ctx, r) }})
	}
	job(catalog.JobSync, core.CatalogEvery, catalog.Sync)
	job(reconcile.JobMinute, time.Minute, reconcile.Minute)
	job(reconcile.JobDaily, 24*time.Hour, reconcile.Daily)
	job(lifecycle.JobPurge, time.Hour, lifecycle.Purge)
	return nil
}
