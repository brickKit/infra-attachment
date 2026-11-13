// Package core is what every module of the attachment component uses: its names, its resources and operations, the
// states, the parameters, the category snapshot, its errors and checks, and Base.
package core

import (
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
)

// ID is the component's ID.
const ID = "infra/attachment"

// Stream is the component's own stream: the attachments' state (docs/design.md AT13).
const Stream = "infra.attachment"

// Changed is the attachment state event's type (AT13).
const Changed = "infra.attachment.changed.v1"

// The configuration (AT05, AT09).
const (
	// KeyScan picks the scan port: off (development and the cells) or clamav (later).
	KeyScan = "ATTACHMENT_SCAN"
	// KeyRetention gives each retention class its minimum and maximum, in days: a JSON object
	// {"<class>": {"min_days": n, "max_days": m}}; a class it does not give is to be confirmed and never purged.
	KeyRetention = "ATTACHMENT_RETENTION"
	// KeyAuthzURL is the authorization member's system surface: the category catalog (AT01).
	KeyAuthzURL = "AUTHZ_GRPC_URL"
)

// Resources and operations (contract/openapi.yaml and the proto name each one's operation).
const (
	Uploads           = "infra.attachment.upload"
	OpConfirm         = "infra.attachment.upload.confirm" // every user: the credential the owner issued bounds it
	Downloads         = "infra.attachment.download"
	OpDownload        = "infra.attachment.download.read" // every user: the credential the owner issued bounds it
	System            = "infra.attachment.system"
	OpManage          = "infra.attachment.system.manage" // any component: the owner check is AT02
	OpRecover         = "infra.attachment.system.recover"
	OpResend          = "infra.attachment.system.resend"
	ScanOff           = "off"
	SensitivityNormal = "normal"
	SensitivityStrict = "strict"
)

// The attachment states (docs/design.md, the state table).
const (
	Uploading   = "uploading"
	Finalizing  = "finalizing"
	PendingScan = "pending_scan"
	Available   = "available"
	Rejected    = "rejected"
	Quarantined = "quarantined"
	Missing     = "missing"
	Detached    = "detached"
	Purged      = "purged"
)

// The version states.
const (
	VUploading   = "uploading"
	VFinalizing  = "finalizing"
	VPendingScan = "pending_scan"
	VClean       = "clean"
	VRejected    = "rejected"
	VQuarantined = "quarantined"
	VSuperseded  = "superseded"
)

// The credential operations.
const (
	CredConfirm  = "confirm"
	CredDownload = "download"
)

// Parameters (assembly protocol 12's starting values).
const (
	UploadTTL       = 15 * time.Minute
	DownloadTTL     = 5 * time.Minute
	StrictTTL       = time.Minute
	FinalizeTimeout = 10 * time.Minute
	Grace           = 30 * 24 * time.Hour
	OrphanWindow    = 7 * 24 * time.Hour
	DeadVersionKept = 30 * 24 * time.Hour
	ScanAlertAfter  = time.Hour
	CatalogEvery    = time.Minute
	CatalogStale    = 15 * time.Minute
	// MaxBytes is the component's own limit: no category goes beyond it.
	MaxBytes = 100 << 20
)

// Category is a category of the catalog, as a new attachment snapshots it (AT01, AT03).
type Category struct {
	ResourceType   string   `json:"-"`
	Owner          string   `json:"-"`
	Name           string   `json:"-"`
	Field          string   `json:"field"`
	MaxBytes       int64    `json:"max_bytes"`
	AllowedTypes   []string `json:"allowed_types"`
	Sensitivity    string   `json:"sensitivity"`
	RetentionClass string   `json:"retention_class"`
	Version        string   `json:"version"`
}

// Definitions are the component's resources and operations; none has a dimension (the owners judge the users).
func Definitions() *sdkauthz.Definitions {
	none := map[string]sdkauthz.Dimension{}
	f := map[string]sdkauthz.Field{}
	return &sdkauthz.Definitions{Component: ID,
		Resources: []sdkauthz.Resource{
			{Type: Uploads, ViewOperation: OpConfirm, Dimensions: none, Fields: f},
			{Type: Downloads, ViewOperation: OpDownload, Dimensions: none, Fields: f},
			{Type: System, ViewOperation: OpManage, Dimensions: none, Fields: f},
		},
		Operations: []sdkauthz.Operation{
			{Key: OpConfirm, ResourceType: Uploads, Purpose: "action", RiskClass: "low", SideEffects: []string{"business_write"}, Audit: true},
			{Key: OpDownload, ResourceType: Downloads, Purpose: "read", RiskClass: "low", Audit: true},
			{Key: OpManage, ResourceType: System, Purpose: "action", RiskClass: "low", SideEffects: []string{"business_write"}},
			{Key: OpRecover, ResourceType: System, Purpose: "read", RiskClass: "low"},
			{Key: OpResend, ResourceType: System, Purpose: "read", RiskClass: "low"},
		}}
}
