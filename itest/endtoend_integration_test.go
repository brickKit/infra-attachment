//go:build integration

package itest

// The attachment component against PostgreSQL, the bus and the object store (RustFS) over its real surfaces (L2).
// The owners are the test itself: it calls the system surface over mTLS as demo/orders (the owner of demo.order) and
// as demo/other; the category catalog is a stand-in the test sets (the authorization member's catalog needs the
// owners' definitions). The users upload and download through the presigned addresses, as a browser would. Needs
// BE_SDK_TEST_PG_ADMIN, BE_SDK_TEST_NATS_URL and BE_SDK_TEST_S3_*. The member listens on 28204 (user) and 9204
// (system). Run: go test -tags integration ./itest/ (make test-integration); the index of the steps is
// itest/AGENTS.md.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	attachment "github.com/brickKit/infra-attachment/v3/attachment"
	core "github.com/brickKit/infra-attachment/v3/internal/core"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/events/bus"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/brickKit/be-sdk-go/v3/storage"
	attachmentv3 "github.com/brickKit/infra-attachment/contract/v3/gen/go/be/attachment/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	itPort   = "28204"
	itBase   = "http://localhost:" + itPort
	itSystem = "localhost:9204"
	itSchema = "infra_attachment_it"
	owner    = "demo/orders"
	carol    = "0190f3a1-0000-7000-8000-0000000000c3" // a user with the attachment routes
	dave     = "0190f3a1-0000-7000-8000-0000000000d4" // no grant at all
)

// catalog is the category catalog the test sets (core.CatalogSource): demo.order of demo/orders, a normal and a
// strict category.
type catalog struct{}

func (catalog) Fetch(context.Context, string) (string, bool, []core.Category, error) {
	return "r1", false, []core.Category{
		{ResourceType: "demo.order", Owner: owner, Name: "demo.order_contract", Field: "contract_file", MaxBytes: 1 << 20,
			AllowedTypes: []string{"application/pdf", "image/png"}, Sensitivity: "normal", RetentionClass: "demo_contract", Version: "1"},
		{ResourceType: "demo.order", Owner: owner, Name: "demo.order_invoice", Field: "invoice_scan", MaxBytes: 1 << 20,
			AllowedTypes: []string{"application/pdf"}, Sensitivity: "strict", RetentionClass: "demo_invoice", Version: "1"},
	}, nil
}

// fixture is the running member and the world, shared by the steps.
type fixture struct {
	t      *testing.T
	ctx    context.Context
	w      *sectest.World
	admin  *pgx.Conn
	store  *storage.Store // the test's own view of the member's prefix: to tamper with
	owner  attachmentv3.AttachmentServiceClient
	other  attachmentv3.AttachmentServiceClient
	bob    client
	carol  client
	dave   client
	record string
}

func TestAttachmentEndToEnd(t *testing.T) {
	fx := start(t)
	fx.upload()
	fx.credentials()
	fx.content()
	fx.owners()
	fx.strict()
	fx.replace()
	fx.lifecycle()
	fx.missing()
}

func start(t *testing.T) *fixture {
	adminURL, natsURL := os.Getenv("BE_SDK_TEST_PG_ADMIN"), os.Getenv("BE_SDK_TEST_NATS_URL")
	if adminURL == "" || natsURL == "" {
		t.Fatal("BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL are needed")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close(ctx) })
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)
	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()
	w := sectest.New(t)
	for _, who := range []string{sectest.Bob, carol} { // every user confirms and downloads with the owners' credentials
		for _, op := range []struct{ res, op string }{{core.Uploads, core.OpConfirm}, {core.Downloads, core.OpDownload}} {
			w.Grant(who, sdkauthz.Segment{ResourceType: op.res, Operation: op.op, Atoms: map[string]sdkauthz.Atom{},
				Valid: sdkauthz.Window{From: "2026-01-01T00:00:00Z"}})
		}
	}
	fx := &fixture{t: t, ctx: ctx, w: w, admin: admin, record: ids.New().String()}
	s3url, access, secret := os.Getenv("BE_SDK_TEST_S3_URL"), os.Getenv("BE_SDK_TEST_S3_ACCESS_KEY"), os.Getenv("BE_SDK_TEST_S3_SECRET_KEY")
	require.NotEmpty(t, s3url, "BE_SDK_TEST_S3_URL, BE_SDK_TEST_S3_ACCESS_KEY and BE_SDK_TEST_S3_SECRET_KEY are needed")
	prefix := fmt.Sprintf("attachment-it-%d/", time.Now().UnixNano())
	fx.store, err = storage.New(s3url, "us-east-1", "be-sdk-test", prefix, access, func() string { return secret }, nil)
	require.NoError(t, err)
	portFree(t, itPort, itSystem)

	tr, err := bus.Open(ctx, natsURL, "attachment-it", slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { tr.Close() })
	require.NoError(t, tr.EnsureStream(ctx, events.Stream{Name: core.Stream, Partitions: 4}))

	env := w.Env(t, core.ID)
	for k, v := range map[string]string{"COMPONENT_ID": core.ID, "COMPONENT_VERSION": attachment.Version, "PORT": itPort,
		"STOP_DRAIN_DELAY": "50ms", "PG_HOST": u.Hostname(), "PG_PORT": u.Port(), "PG_DATABASE": strings.TrimPrefix(u.Path, "/"),
		"PG_USER": u.User.Username(), "PG_PASSWORD": pw, "PG_SCHEMA": itSchema, "PG_OWNER_USER": u.User.Username(),
		"PG_OWNER_PASSWORD": pw, "BUS_URL": natsURL, "IMPLEMENTATION_INSTANCE": ids.New().String(),
		"OBJECT_STORE_URL": s3url, "OBJECT_STORE_BUCKET": "be-sdk-test", "OBJECT_STORE_PREFIX": prefix,
		"OBJECT_STORE_ACCESS_KEY": access, "OBJECT_STORE_SECRET_KEY": secret,
		core.KeyRetention: `{"demo_contract": {"min_days": 0, "max_days": 3650}}`} {
		env[k] = v
	}
	var logs bytes.Buffer
	var logMu sync.Mutex
	t.Cleanup(func() {
		if t.Failed() {
			logMu.Lock()
			t.Log(logs.String())
			logMu.Unlock()
		}
	})
	out := &lockedWriter{w: &logs, mu: &logMu}
	opts := launcher.Options{Stdout: out, Stderr: out,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	migrate := opts
	migrate.Args = []string{"migrate"}
	def := attachment.Definition()
	newModule := def.New
	def.New = func(rt *runtime.Runtime) (component.Module, error) {
		m, err := newModule(rt)
		if err == nil {
			m.(*attachment.Module).UseCatalog(catalog{})
		}
		return m, err
	}
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, def, migrate))
	sctx, stop := context.WithCancel(ctx)
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, def, opts) }()
	t.Cleanup(func() { stop(); <-done })
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 20*time.Second, 100*time.Millisecond)
	fx.owner = attachmentv3.NewAttachmentServiceClient(fx.dial(owner))
	fx.other = attachmentv3.NewAttachmentServiceClient(fx.dial("demo/other"))
	fx.bob = client{t, w.Bearer(sectest.Bob)}
	fx.carol = client{t, w.Bearer(carol)}
	fx.dave = client{t, w.Bearer(dave)}
	return fx
}

// dial is a component's mTLS connection to the member's system surface.
func (fx *fixture) dial(as string) *grpc.ClientConn {
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(fx.w.TLS(fx.t, as).ClientConfig(core.ID))))
	require.NoError(fx.t, err)
	fx.t.Cleanup(func() { conn.Close() })
	return conn
}

func (fx *fixture) parent() *attachmentv3.Parent {
	return &attachmentv3.Parent{ResourceType: "demo.order", RecordId: fx.record}
}

// put uploads through a presigned address as a browser would.
func (fx *fixture) put(u, contentType string, body []byte) int {
	fx.t.Helper()
	req, _ := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(fx.t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// get downloads through a presigned address.
func (fx *fixture) get(u string) (int, []byte, http.Header) {
	fx.t.Helper()
	resp, err := http.Get(u)
	require.NoError(fx.t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

// pdf is a small PDF the server detects as application/pdf.
func pdf(text string) []byte { return []byte("%PDF-1.4\n%" + text + "\n%%EOF\n") }

// --- helpers ---

type client struct {
	t     *testing.T
	token string
}

func (c client) do(method, path string, body any) (int, map[string]any) {
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
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// portFree fails at once when a port the member needs is taken (a running cell or another member would answer).
func portFree(t *testing.T, addrs ...string) {
	for _, a := range addrs {
		if !strings.Contains(a, ":") {
			a = "localhost:" + a
		}
		ln, err := net.Listen("tcp", a)
		require.NoError(t, err, fmt.Sprintf("port %s is taken: is a cell or another member running?", a))
		_ = ln.Close()
	}
}
