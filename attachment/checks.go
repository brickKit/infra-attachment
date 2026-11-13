package attachment

import "github.com/brickKit/be-sdk-go/v3/recovery"

// RecoveryChecks registers what a full recovery asks this member (be-protocol chapter 14): nothing of its own in this
// version. Its audit records carry no object version to check, and it numbers nothing; that every record's final
// object exists after a restore (assembly protocol 12, FA18) is the daily reconciliation's, until the full recovery
// drill (C14) adds it here.
func (m *Module) RecoveryChecks(c *recovery.Checks) error { return nil }
