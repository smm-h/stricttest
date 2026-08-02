package hygiene

import (
	"fmt"
	"testing"
)

// recordingTB stands in for a test's TB so the package's own failure paths can
// be exercised without failing the real test. Errorf and Fatalf are recorded
// instead of reported, and Cleanup functions are held so a test can run them at
// a chosen moment.
//
// The embedded testing.TB is the real *testing.T: TB carries an unexported
// method, so an external implementation is impossible, and embedding is the
// only way to satisfy the interface. Everything not overridden below (Setenv,
// TempDir, Log, ...) therefore behaves normally and stays bound to the real
// test's lifetime.
type recordingTB struct {
	testing.TB
	errors   []string
	fatals   []string
	cleanups []func()
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	// Deliberately does NOT abort: every production call site returns
	// immediately after Fatalf, so recording is enough to observe the failure.
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Cleanup(f func()) {
	r.cleanups = append(r.cleanups, f)
}

// runCleanups runs the recorded cleanups in the LIFO order testing uses.
func (r *recordingTB) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
	r.cleanups = nil
}
