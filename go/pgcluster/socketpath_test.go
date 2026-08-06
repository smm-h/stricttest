// The socket-path guard and the argument validation around the cluster.
//
// None of these tests need PostgreSQL: they cover the refusals that must happen
// before a single binary is executed.

package pgcluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSocketPathIsThePostgresSocketFile(t *testing.T) {
	if got := SocketPathFor("/dev/shm/stpg-abc", 5432); got != "/dev/shm/stpg-abc/.s.PGSQL.5432" {
		t.Errorf("SocketPathFor = %q", got)
	}
}

func TestAShortSocketDirPassesAndReturnsTheSocketPath(t *testing.T) {
	path, err := CheckSocketDir("/dev/shm/stpg-abc", 5432)
	if err != nil {
		t.Fatalf("CheckSocketDir: %v", err)
	}
	if path != "/dev/shm/stpg-abc/.s.PGSQL.5432" {
		t.Errorf("CheckSocketDir = %q", path)
	}
}

// dirOfSocketPathLength returns a directory whose socket path is exactly length
// bytes.
func dirOfSocketPathLength(t *testing.T, length, port int) string {
	t.Helper()
	suffix := "/" + fmt.Sprintf(socketFileTemplate, port)
	dir := "/d" + strings.Repeat("x", length-len(suffix)-2)
	if len(dir+suffix) != length {
		t.Fatalf("built a %d-byte socket path, wanted %d", len(dir+suffix), length)
	}
	return dir
}

func TestTheLimitIsExactAtTheBoundary(t *testing.T) {
	atLimit := dirOfSocketPathLength(t, SUNPathMax, 5432)
	path, err := CheckSocketDir(atLimit, 5432)
	if err != nil {
		t.Fatalf("a path of exactly SUNPathMax bytes was refused: %v", err)
	}
	if len(path) != SUNPathMax {
		t.Fatalf("checked path is %d bytes, wanted %d", len(path), SUNPathMax)
	}

	oneOver := dirOfSocketPathLength(t, SUNPathMax+1, 5432)
	_, err = CheckSocketDir(oneOver, 5432)
	if !errors.Is(err, ErrSocketPathTooLong) {
		t.Fatalf("one byte over the limit gave %v, wanted ErrSocketPathTooLong", err)
	}
	message := err.Error()
	for _, want := range []string{
		strconv.Itoa(SUNPathMax),
		strconv.Itoa(SUNPathMax + 1),
		// The remediation must name the escape route, not just the problem.
		"SocketParent",
		"/dev/shm",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, message)
		}
	}
}

func TestThePortCountsTowardTheLimit(t *testing.T) {
	// The socket file name carries the port, so a longer port shortens the dir.
	dir := dirOfSocketPathLength(t, SUNPathMax, 5432)
	if _, err := CheckSocketDir(dir, 54321); !errors.Is(err, ErrSocketPathTooLong) {
		t.Fatalf("a longer port gave %v, wanted ErrSocketPathTooLong", err)
	}
}

func TestASocketDirWithWhitespaceIsRefused(t *testing.T) {
	_, err := CheckSocketDir("/dev/shm/stpg abc", 5432)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v, wanted ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "whitespace") {
		t.Errorf("the refusal does not say why:\n%s", err)
	}
}

func TestAnExplicitSocketParentThatIsTooLongIsAnErrorNotAFallback(t *testing.T) {
	// An explicit choice is never silently replaced by a working one. The
	// directory below really exists and really is writable -- the only thing
	// wrong with it is its length, so a fallback would be indistinguishable from
	// success.
	tooLong := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	if err := os.Mkdir(tooLong, 0o700); err != nil {
		t.Fatalf("creating the over-long parent: %v", err)
	}
	cluster, err := Start("TEST_DSN", SocketParent(tooLong))
	if cluster != nil {
		cluster.Stop()
		t.Fatal("an over-long explicit socket parent started a cluster anyway")
	}
	if !errors.Is(err, ErrSocketPathTooLong) && !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("got %v, wanted ErrSocketPathTooLong", err)
	}

	// A parent that does not exist is equally an error rather than a fallback.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cluster, err = Start("TEST_DSN", SocketParent(missing))
	if cluster != nil {
		cluster.Stop()
		t.Fatal("a missing explicit socket parent started a cluster anyway")
	}
	if !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("got %v, wanted ErrPostgresUnavailable", err)
	}
}

func TestDSNEnvHasNoDefault(t *testing.T) {
	cluster, err := Start("")
	if cluster != nil {
		cluster.Stop()
		t.Fatal("an empty dsnEnv started a cluster")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v, wanted ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "no default name") {
		t.Errorf("the refusal does not say there is no default:\n%s", err)
	}
}

func TestDatabaseNamesOutsideTheClosedCharacterSetAreRefused(t *testing.T) {
	for _, name := range []string{
		`evil"; DROP DATABASE postgres; --`,
		"has space",
		"1leading_digit",
		"has-dash",
		"",
		strings.Repeat("x", 64),
	} {
		t.Run(name, func(t *testing.T) {
			err := checkName(name)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got %v, wanted ErrInvalidArgument", err)
			}
			if !strings.Contains(err.Error(), "not accepted") {
				t.Errorf("the refusal does not say the name was rejected:\n%s", err)
			}
		})
	}
}

func TestOrdinaryDatabaseNamesAreAccepted(t *testing.T) {
	for _, name := range []string{"test_a", "_x", "T1", strings.Repeat("x", 63)} {
		t.Run(name, func(t *testing.T) {
			if err := checkName(name); err != nil {
				t.Fatalf("%q was refused: %v", name, err)
			}
		})
	}
}

func TestGeneratedNamesAreUniqueAndAcceptable(t *testing.T) {
	names := make(map[string]bool, 50)
	for i := 0; i < 50; i++ {
		name := GenerateName()
		if names[name] {
			t.Fatalf("GenerateName repeated %q", name)
		}
		names[name] = true
		if err := checkName(name); err != nil {
			t.Fatalf("GenerateName produced an unacceptable name %q: %v", name, err)
		}
	}
}

// noBinaryOnPath is a LookPath that finds nothing, standing in for a machine
// whose PATH carries no PostgreSQL program.
func noBinaryOnPath(name string) (string, error) {
	return "", fmt.Errorf("%s: not found in $PATH", name)
}

func TestMissingBinariesReportPreciselyWhatIsMissing(t *testing.T) {
	_, err := findBinaries(noBinaryOnPath, nil, nil)
	if !errors.Is(err, ErrPostgresUnavailable) {
		t.Fatalf("got %v, wanted ErrPostgresUnavailable", err)
	}
	message := err.Error()
	for _, binary := range requiredBinaries {
		if !strings.Contains(message, binary) {
			t.Errorf("the failure does not name %s:\n%s", binary, message)
		}
	}
	if !strings.Contains(message, "server package") {
		t.Errorf("the failure does not say the server package is needed:\n%s", message)
	}
}

func TestBinaryDiscoveryFindsAFedoraStyleUsrBinLayout(t *testing.T) {
	// No PATH entry, no versioned directory, no pg_ctlcluster wrapper.
	bindir := filepath.Join(t.TempDir(), "usr", "bin")
	if err := os.MkdirAll(bindir, 0o755); err != nil {
		t.Fatalf("creating the fake bindir: %v", err)
	}
	for _, name := range requiredBinaries {
		if err := os.WriteFile(filepath.Join(bindir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("writing the fake %s: %v", name, err)
		}
	}

	binaries, err := findBinaries(noBinaryOnPath, []string{bindir}, nil)
	if err != nil {
		t.Fatalf("findBinaries: %v", err)
	}
	for _, want := range []struct{ got, name string }{
		{binaries.Initdb, "initdb"},
		{binaries.PgCtl, "pg_ctl"},
		{binaries.Psql, "psql"},
	} {
		if want.got != filepath.Join(bindir, want.name) {
			t.Errorf("%s resolved to %q", want.name, want.got)
		}
	}
	if binaries.BinDir() != bindir {
		t.Errorf("BinDir = %q, wanted %q", binaries.BinDir(), bindir)
	}
}

// TestVersionGlobbedDirectoriesAreSearchedInDescendingOrder has no Python
// analogue: the descending sort is a pinned contract (a machine with several
// major versions installed must get the newer one, and the answer must never
// depend on the order the filesystem returns) and nothing else covers it.
//
// The sort is lexical over directory names, not over parsed version numbers, so
// it orders equal-width versions correctly (17 above 16 above 14) and would
// order a single-digit one above all of them. That is the Python
// implementation's behavior too, and the versions this matters for (PostgreSQL
// 9.x) predate every layout in the search list.
func TestVersionGlobbedDirectoriesAreSearchedInDescendingOrder(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"14", "17", "16"} {
		dir := filepath.Join(root, "pgsql-"+version, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
		for _, name := range requiredBinaries {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatalf("writing the fake %s: %v", name, err)
			}
		}
	}
	binaries, err := findBinaries(noBinaryOnPath, []string{filepath.Join(root, "pgsql-*", "bin")}, nil)
	if err != nil {
		t.Fatalf("findBinaries: %v", err)
	}
	if want := filepath.Join(root, "pgsql-17", "bin"); binaries.BinDir() != want {
		t.Errorf("BinDir = %q, wanted %q", binaries.BinDir(), want)
	}
}
