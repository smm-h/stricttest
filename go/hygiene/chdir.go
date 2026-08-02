package hygiene

import (
	"os"
	"testing"
)

// Chdir moves the process working directory to dir for the duration of t and
// restores the previous one when t finishes.
//
// A failed restore FAILS the test rather than being logged and swallowed: the
// process is then sitting in the wrong directory, and every later test in the
// binary would run against it. A test that reports the damage is the only
// honest outcome.
//
// The process working directory is global state, so this -- like the rest of
// the package -- is incompatible with T.Parallel. PWD is updated alongside the
// real working directory so that child processes inheriting the environment
// agree with the parent about where they are.
func Chdir(t testing.TB, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("hygiene: reading the current working directory before chdir to %s: %v", dir, err)
		return
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("hygiene: chdir to %s: %v", dir, err)
		return
	}
	// Setenv (not os.Setenv) so PWD is restored with everything else, and so a
	// parallel test trips the same panic the rest of the package relies on.
	if resolved, err := os.Getwd(); err == nil {
		t.Setenv("PWD", resolved)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("hygiene: restoring the working directory to %s after the "+
				"test chdir'd to %s: %v -- every later test in this binary now "+
				"runs from the wrong directory", previous, dir, err)
		}
	})
}
