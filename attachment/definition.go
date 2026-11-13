// Package attachment is the attachment component (assembly protocol 12): the attachments of the owners' records, their
// upload sessions and downloads by short-lived credentials the owners ask for, the content checked as it was copied to
// a final key, and their lifecycle by the owners' commands. It never calls an owner (D68).
package attachment

import (
	"embed"
	"io/fs"

	core "github.com/brickKit/infra-attachment/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/storage"
)

// Identity.
const (
	Version  = "3.0.4"
	GRPCPort = 9204
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Keys are the component's own configuration keys, with the storage port's.
func Keys() []config.Key {
	return append([]config.Key{
		{Name: core.KeyScan, Type: config.String, Default: core.ScanOff},
		{Name: core.KeyRetention, Type: config.String},
	}, storage.Keys()...)
}

// Own describes the own keys for the manifest's configSchema.
var Own = map[string]string{
	core.KeyScan: "the scan port: off (the attachments are available unscanned); this version has no engine yet",
	core.KeyRetention: "each retention class's minimum and maximum in days, a JSON object {\"<class>\": {\"min_days\": n, " +
		"\"max_days\": m}}; a class it does not give is to be confirmed and never purged",
	storage.KeyPublicURL: "the object store's address as browsers reach it: the presigned upload and download addresses " +
		"are signed for it; unset: OBJECT_STORE_URL",
}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: core.ID, Version: Version, Keys: Keys(), Migrations: migrations(), GRPCPort: GRPCPort,
		Streams: []events.Stream{{Name: core.Stream, Partitions: 4}}, Authz: core.Definitions(), Recovery: core.OpRecover,
		Resend: core.OpResend, New: func(rt *runtime.Runtime) (component.Module, error) { return New(rt) }}
}
