// Command seed fills a running infra/attachment with the seed data for people (be-assembly-standard's seed and test
// data conventions). The attachment component owns no data of its own to seed: the attachments are the owners' (the
// purchase orders' contracts are erp/purchase's seed). It grants, by the authorization admin, the role attachments to
// the seed user, the purchasing and finance leads, the buyers and the cashier: confirming their uploads and
// downloading strict categories, each bounded by the credential an owner issues.
//
// A second run changes nothing. It waits until the grants reach the component (a confirmation with no credential
// answers 400, not 404: the operation is checked before the body).
//
//	SEED_IAM_URL (http://localhost:18200)    the identity member's development exchange
//	SEED_AUTHZ_URL (http://localhost:18223)  the authorization member's effect point
//	SEED_ADMIN (test.alice)                  the authorization admin (the install claim)
//	SEED_USER (test.bob), SEED_PASSWORD      the seed user; the test accounts' password
//	INFRA_ATTACHMENT_URL (http://localhost:18204)
package main

import (
	"fmt"
	"os"
	"time"

	sdkauthz "github.com/brickKit/be-sdk-go/v3/authz"
	"github.com/brickKit/be-sdk-go/v3/seed"
)

const prefix = "「本地测试」"

// users are the accounts that get an inbox: the approvers and the ones who submit and follow.
var users = []string{"test.buyer.lead", "test.finance.lead", "test.buyer.cn", "test.cashier.cn"}

func main() {
	base := env("INFRA_ATTACHMENT_URL", "http://localhost:18204")
	if !seed.Ready(base) {
		fail("infra/attachment at %s is not ready", base)
	}
	pw := os.Getenv("SEED_PASSWORD")
	if pw == "" {
		fail("SEED_PASSWORD is not set (the test accounts' password in Casdoor)")
	}
	iam, user, admin := env("SEED_IAM_URL", "http://localhost:18200"), env("SEED_USER", "test.bob"), env("SEED_ADMIN", "test.alice")
	accounts, err := seed.Accounts(iam, pw, append([]string{admin, user}, users...)...)
	check(err)
	a, err := seed.NewAuthz(env("SEED_AUTHZ_URL", "http://localhost:18223"), accounts[admin].Token, "infra/attachment")
	check(err)
	segs := []sdkauthz.Segment{
		segment("infra.attachment.upload", "infra.attachment.upload.confirm"),
		segment("infra.attachment.download", "infra.attachment.download.read"),
	}
	_, err = a.GrantRoles([]seed.RoleSpec{
		{Key: "attachments", Name: prefix + "附件", To: append([]string{user}, users...), Segments: segs},
	}, accounts)
	check(err)
	for _, u := range append([]string{user}, users...) {
		check(seed.NewClient(base, accounts[u].Token).WaitGranted("POST", "/api/attachment/uploads/confirm", time.Minute))
	}
	fmt.Printf("seed infra/attachment: the attachment routes granted to %d accounts\n", len(users)+1)
}

// segment grants an operation on a resource of the attachment component (no dimension: the credential bounds it).
func segment(resource, op string) sdkauthz.Segment {
	s := seed.Segment(resource, op)
	s.Atoms = map[string]sdkauthz.Atom{}
	return s
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "seed infra/attachment: "+format+"\n", a...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}
