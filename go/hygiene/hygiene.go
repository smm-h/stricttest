// Package hygiene provides test-environment isolation helpers for Go suites:
// a throwaway HOME, an isolated git config and identity, transport lockdown,
// credential stripping, and cleanup-restoring chdir.
//
// The helper surface is not implemented yet. This file exists so the module is
// buildable and resolvable from its first release; the surface lands in its own
// change.
package hygiene

// Version is the module's release version. It is kept in step with the VERSION
// file at the module root.
const Version = "0.1.0"
