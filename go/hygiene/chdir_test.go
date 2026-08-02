package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolved works around macOS, where TempDir hands back a /var path that is a
// symlink to /private/var: after chdir, Getwd reports the resolved form.
func resolved(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving %s: %v", path, err)
	}
	return out
}

func TestChdirMovesTheProcessAndRestoresIt(t *testing.T) {
	before, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	target := t.TempDir()

	t.Run("during", func(t *testing.T) {
		Chdir(t, target)
		got, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		if want := resolved(t, target); got != want {
			t.Errorf("working directory = %q, want %q", got, want)
		}
		if pwd := os.Getenv("PWD"); pwd != got {
			t.Errorf("PWD = %q, want %q -- child processes would disagree with the parent", pwd, got)
		}
	})

	after, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if after != before {
		t.Errorf("working directory = %q after the subtest, want %q", after, before)
	}
}

func TestChdirNestsAndUnwindsInOrder(t *testing.T) {
	outer := t.TempDir()
	inner := t.TempDir()

	t.Run("outer", func(t *testing.T) {
		Chdir(t, outer)
		t.Run("inner", func(t *testing.T) {
			Chdir(t, inner)
			got, _ := os.Getwd()
			if want := resolved(t, inner); got != want {
				t.Errorf("working directory = %q, want %q", got, want)
			}
		})
		got, _ := os.Getwd()
		if want := resolved(t, outer); got != want {
			t.Errorf("working directory = %q after the inner subtest, want %q", got, want)
		}
	})
}

func TestChdirFailsTheTestWhenItCannotMove(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	rec := &recordingTB{TB: t}
	Chdir(rec, missing)
	if len(rec.fatals) != 1 {
		t.Fatalf("expected exactly one failure, got %v", rec.fatals)
	}
	if !strings.Contains(rec.fatals[0], missing) {
		t.Errorf("failure message does not name the directory: %q", rec.fatals[0])
	}
	if len(rec.cleanups) != 0 {
		t.Error("a restore cleanup was registered even though the chdir failed")
	}
}

// The restore-failure path: the directory the test started in disappears while
// the test is running, so the cleanup cannot get back. The process is then
// sitting somewhere unexpected, and the test must say so.
func TestChdirFailsTheTestWhenTheRestoreFails(t *testing.T) {
	// Move the real test into a directory that is about to be deleted. This
	// Chdir is bound to the real t, so the genuine working directory is
	// restored when this test ends regardless of what happens below.
	doomed := filepath.Join(t.TempDir(), "doomed")
	if err := os.Mkdir(doomed, 0o700); err != nil {
		t.Fatalf("creating %s: %v", doomed, err)
	}
	Chdir(t, doomed)

	elsewhere := t.TempDir()
	rec := &recordingTB{TB: t}
	Chdir(rec, elsewhere)

	if err := os.RemoveAll(doomed); err != nil {
		t.Fatalf("removing %s: %v", doomed, err)
	}
	rec.runCleanups()

	if len(rec.errors) != 1 {
		t.Fatalf("expected exactly one failure from the failed restore, got %v", rec.errors)
	}
	if !strings.Contains(rec.errors[0], "restoring the working directory") {
		t.Errorf("failure message does not explain what went wrong: %q", rec.errors[0])
	}
}
