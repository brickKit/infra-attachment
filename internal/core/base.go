package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/brickKit/be-sdk-go/v3/runtime"
	"github.com/brickKit/be-sdk-go/v3/storage"
)

// Base is what every module works with: the component's runtime, its object store, the category catalog's source,
// the retention classes and the scan port.
type Base struct {
	RT        *runtime.Runtime
	Store     *storage.Store
	Catalog   CatalogSource
	Retention map[string]Retention
	Scan      string
}

// Retention is a retention class's minimum and maximum (AT09).
type Retention struct {
	MinDays int `json:"min_days"`
	MaxDays int `json:"max_days"`
}

// CatalogSource answers the attachment categories of the accepted definitions (AT01): the authorization member's
// CatalogService; the integration tests set their own.
type CatalogSource interface {
	Fetch(ctx context.Context, known string) (revision string, unchanged bool, categories []Category, err error)
}

// NewSecret is a credential's value (an opaque random token) and the digest the component keeps of it.
func NewSecret() (value, digest string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	value = base64.RawURLEncoding.EncodeToString(b)
	return value, Digest(value)
}

// Digest is what the component keeps of a credential.
func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
