// The TB-bound failure paths.
//
// [Ephemeral] and [Cluster.Database] fail the test instead of returning an
// error, which makes them the two entry points whose refusals cannot be
// observed by an ordinary assertion. They are driven here through recordingTB,
// which records Fatalf instead of aborting, so the failure a real consumer
// would see -- its message, and the zero value returned after it -- is asserted
// like any other value.
//
// None of these tests need a running cluster: every configuration below is
// impossible on any machine, so they behave the same with PostgreSQL installed
// and without it.

package pgcluster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBinaries are binary paths that certainly do not exist, so discovery is
// skipped (the cluster believes it already has its binaries) and any attempt to
// actually run one fails at exec time rather than reaching a real postmaster.
func fakeBinaries(t *testing.T) Binaries {
	t.Helper()
	dir := t.TempDir()
	return Binaries{
		Initdb: filepath.Join(dir, "initdb"),
		PgCtl:  filepath.Join(dir, "pg_ctl"),
		Psql:   filepath.Join(dir, "psql"),
	}
}

func TestEphemeralFailsTheTestWhenTheArgumentsAreRefused(t *testing.T) {
	// The pre-flight refusal: newCluster rejects the empty dsnEnv, so the test
	// fails before anything is picked, created or executed.
	rec := &recordingTB{TB: t}
	c := Ephemeral(rec, "")

	if c != nil {
		c.Stop()
		t.Fatal("an empty dsnEnv produced a cluster")
	}
	if len(rec.fatals) != 1 {
		t.Fatalf("expected exactly one failure, got %v", rec.fatals)
	}
	if !strings.HasPrefix(rec.fatals[0], "pgcluster: ") {
		t.Errorf("failure message is not attributed to this package: %q", rec.fatals[0])
	}
	if !strings.Contains(rec.fatals[0], "no default name") {
		t.Errorf("failure message does not say there is no default: %q", rec.fatals[0])
	}
	if len(rec.cleanups) != 0 {
		t.Error("a shutdown cleanup was registered even though no cluster was created")
	}
}

func TestEphemeralFailsTheTestWhenTheClusterCannotStart(t *testing.T) {
	if os.Geteuid() == 0 {
		// A root process is refused earlier, with a different message, so the
		// path under test here is never reached.
		t.Skip("PostgreSQL refuses to run as root, so start() fails for another reason")
	}
	// An explicit socket parent that does not exist is never second-guessed, so
	// this configuration cannot start on any machine. The binaries are supplied
	// so discovery is skipped and the refusal is unambiguously this one.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	rec := &recordingTB{TB: t}
	c := Ephemeral(rec, "TEST_DSN", SocketParent(missing), UseBinaries(fakeBinaries(t)))

	if c != nil {
		c.Stop()
		t.Fatal("a missing explicit socket parent produced a cluster")
	}
	if len(rec.fatals) != 1 {
		t.Fatalf("expected exactly one failure, got %v", rec.fatals)
	}
	if !strings.HasPrefix(rec.fatals[0], "pgcluster: ") {
		t.Errorf("failure message is not attributed to this package: %q", rec.fatals[0])
	}
	if !strings.Contains(rec.fatals[0], missing) {
		t.Errorf("failure message does not name the parent it refused: %q", rec.fatals[0])
	}
	if len(rec.cleanups) != 0 {
		t.Error("a shutdown cleanup was registered even though the start failed")
	}
	// The DSN variable must not have been touched: Ephemeral exports it only
	// after a successful start.
	if _, ok := os.LookupEnv("TEST_DSN"); ok {
		t.Error("TEST_DSN was exported even though the cluster never started")
	}
}

func TestDatabaseFailsTheTestWhenTheDatabaseCannotBeCreated(t *testing.T) {
	// A cluster that was never started: psql cannot be run, so CREATE DATABASE
	// fails the way it would against a cluster that has already been stopped,
	// and the per-test database path has to fail the test rather than hand back
	// a URL that points at nothing.
	c, err := newCluster("TEST_DSN", UseBinaries(fakeBinaries(t)))
	if err != nil {
		t.Fatalf("newCluster: %v", err)
	}
	rec := &recordingTB{TB: t}
	dbURL := c.Database(rec)

	if dbURL != "" {
		t.Errorf("Database returned %q after failing the test", dbURL)
	}
	if len(rec.fatals) != 1 {
		t.Fatalf("expected exactly one failure, got %v", rec.fatals)
	}
	if !strings.HasPrefix(rec.fatals[0], "pgcluster: ") {
		t.Errorf("failure message is not attributed to this package: %q", rec.fatals[0])
	}
	if !strings.Contains(rec.fatals[0], "psql") {
		t.Errorf("failure message does not name the command that failed: %q", rec.fatals[0])
	}
	if len(rec.cleanups) != 0 {
		t.Error("a drop cleanup was registered for a database that was never created")
	}
	if _, ok := os.LookupEnv("TEST_DSN"); ok {
		t.Error("TEST_DSN was exported even though no database was created")
	}
}
