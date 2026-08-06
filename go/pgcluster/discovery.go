package pgcluster

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// The socket-path limit
// ---------------------------------------------------------------------------

// SUNPathMax is the number of usable bytes in a unix socket address.
//
// A unix socket address is a fixed-size sockaddr_un.sun_path char array. Linux
// gives it 108 bytes INCLUDING the NUL terminator, so 107 usable bytes; the
// BSDs are smaller still (104). The limit is enforced by the kernel, not by
// PostgreSQL, and it applies to the full socket FILE path -- the directory plus
// PostgreSQL's own .s.PGSQL.<port> name. Exceeding it fails at bind() time with
// a message that names neither the limit nor the path, which is why this
// package refuses up front instead.
const SUNPathMax = 107

// socketFileTemplate is the socket file PostgreSQL creates inside the socket
// directory.
const socketFileTemplate = ".s.PGSQL.%d"

// probeName is the stand-in directory name used to decide whether a candidate
// parent can hold the socket directory. It is exactly as long as the name
// makeTempDir will produce, so a parent that passes the probe cannot fail on the
// real directory.
const probeName = tempDirPrefixProbe + "xxxxxxxx"

// tempDirPrefixProbe is the shortest prefix makeTempDir is called with. The
// socket directory uses it; the data directory's prefix is longer but is never
// probed, because the data directory has no path-length limit.
const tempDirPrefixProbe = "stpg-"

// defaultSocketParents are the candidate parents for the socket directory, in
// order. Short paths first: the whole point is to stay far below SUNPathMax,
// and a session TMPDIR (which under a sandbox runner can be deeply nested) is
// the last resort.
var defaultSocketParents = []string{"/dev/shm", "/tmp"}

// defaultDataParents are the candidate parents for the data directory, in
// order. tmpfs first -- the data directory of a throwaway cluster should never
// touch a disk.
var defaultDataParents = []string{"/dev/shm"}

// binarySearchDirs are the directories searched for the PostgreSQL binaries
// when PATH does not carry them. Fedora installs the server binaries straight
// into /usr/bin (no per-version directory, no pg_ctlcluster wrapper); Debian and
// the PGDG packages use versioned directories.
var binarySearchDirs = []string{
	"/usr/bin",
	"/usr/local/bin",
	"/usr/lib/postgresql/*/bin",
	"/usr/pgsql-*/bin",
	"/usr/local/pgsql/bin",
	"/opt/homebrew/opt/postgresql*/bin",
}

// requiredBinaries are the PostgreSQL programs this package runs.
var requiredBinaries = []string{"initdb", "pg_ctl", "psql"}

// safeName is a database name this package will create or drop. Deliberately
// narrower than what PostgreSQL accepts: the name is interpolated into SQL, and
// a closed character set is a stronger guarantee than a quoting routine.
var safeName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// The sentinel errors every failure from this package wraps. They exist so a
// TestMain can tell "this machine has no PostgreSQL" (skip the suite) apart from
// "the cluster broke" (fail the suite):
//
//	cluster, err := pgcluster.Start("MYAPP_DATABASE_URL")
//	if errors.Is(err, pgcluster.ErrPostgresUnavailable) {
//		// skip
//	}
var (
	// ErrPostgresUnavailable means no usable PostgreSQL installation was found,
	// or the machine cannot host a cluster (no writable parent directory, or the
	// process is root -- PostgreSQL refuses to run as root). The wrapped message
	// carries the precise reason, suitable for a skip message verbatim.
	ErrPostgresUnavailable = errors.New("PostgreSQL is not usable on this machine")

	// ErrSocketPathTooLong means a socket directory would produce a path past
	// the kernel's sun_path limit. See [SUNPathMax].
	ErrSocketPathTooLong = errors.New("unix socket path exceeds the kernel's sun_path limit")

	// ErrInvalidArgument means a caller-supplied value was refused before
	// anything was executed: an empty dsnEnv, a socket directory containing
	// whitespace, or a database name outside the closed character set.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrCluster means a PostgreSQL program this package ran failed. The
	// wrapped message carries the command, its output, and the server log when
	// one exists -- a bind() failure or a refused start says nothing useful on
	// its own.
	ErrCluster = errors.New("PostgreSQL cluster command failed")
)

// ---------------------------------------------------------------------------
// Binary discovery
// ---------------------------------------------------------------------------

// Binaries holds resolved paths to the PostgreSQL programs this package runs.
type Binaries struct {
	Initdb string
	PgCtl  string
	Psql   string
}

// BinDir is the directory the binaries were found in.
func (b Binaries) BinDir() string { return filepath.Dir(b.Initdb) }

// FindBinaries locates initdb, pg_ctl and psql.
//
// PATH wins; otherwise the layouts in this package's search list are tried in
// order, with version-globbed directories sorted descending so a newer major
// version wins over an older one and the choice never depends on directory
// order. extraDirs is searched before either.
//
// The returned error wraps [ErrPostgresUnavailable] and names exactly what was
// missing and where it was looked for, so a suite can skip with it verbatim.
func FindBinaries(extraDirs ...string) (Binaries, error) {
	return findBinaries(exec.LookPath, binarySearchDirs, extraDirs)
}

// findBinaries is FindBinaries with its two environmental dependencies passed
// in, so the discovery order can be tested without a PostgreSQL installation.
func findBinaries(lookPath func(string) (string, error), patterns, extraDirs []string) (Binaries, error) {
	dirs := searchDirs(extraDirs, patterns)
	resolved := make(map[string]string, len(requiredBinaries))
	var missing []string
	for _, name := range requiredBinaries {
		if path, err := lookPath(name); err == nil && path != "" {
			resolved[name] = path
			continue
		}
		found := ""
		for _, dir := range dirs {
			candidate := filepath.Join(dir, name)
			if isExecutableFile(candidate) {
				found = candidate
				break
			}
		}
		if found == "" {
			missing = append(missing, name)
			continue
		}
		resolved[name] = found
	}
	if len(missing) > 0 {
		where := strings.Join(dirs, ", ")
		if where == "" {
			where = "(no candidate directory exists)"
		}
		return Binaries{}, fmt.Errorf(
			"%w: %s not found on PATH nor in %s. Install the PostgreSQL server "+
				"package (the client package alone is not enough -- initdb and "+
				"pg_ctl ship with the server)",
			ErrPostgresUnavailable, strings.Join(missing, ", "), where)
	}
	return Binaries{
		Initdb: resolved["initdb"],
		PgCtl:  resolved["pg_ctl"],
		Psql:   resolved["psql"],
	}, nil
}

// searchDirs expands the search-dir globs into existing directories, newest
// first within each pattern.
func searchDirs(extraDirs, patterns []string) []string {
	candidates := make([]string, 0, len(extraDirs)+len(patterns))
	candidates = append(candidates, extraDirs...)
	for _, pattern := range patterns {
		if !strings.Contains(pattern, "*") {
			candidates = append(candidates, pattern)
			continue
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		// Descending so a newer major version wins over an older one, and so the
		// choice never depends on directory order.
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		candidates = append(candidates, matches...)
	}
	dirs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if isDir(candidate) {
			dirs = append(dirs, candidate)
		}
	}
	return dirs
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// isWritableDir answers whether a directory can be created inside path. It
// probes by creating and removing one, rather than reading permission bits: the
// bits are the wrong answer under an overlay, a read-only mount, or a full
// tmpfs, all of which are exactly the cases a candidate list exists to skip.
func isWritableDir(path string) bool {
	probe, err := os.MkdirTemp(path, ".stpg-probe-")
	if err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// ---------------------------------------------------------------------------
// The socket-path guard
// ---------------------------------------------------------------------------

// SocketPathFor is the socket file PostgreSQL will create for port in dir.
func SocketPathFor(dir string, port int) string {
	return filepath.Join(dir, fmt.Sprintf(socketFileTemplate, port))
}

// CheckSocketDir validates a socket directory, returning the socket path it
// would produce.
//
// It returns an error wrapping [ErrSocketPathTooLong] when the resulting socket
// path would not fit in sockaddr_un.sun_path, and one wrapping
// [ErrInvalidArgument] when the path contains whitespace (the directory is
// passed to the postmaster inside a space-separated options string, where a
// space would silently split it).
func CheckSocketDir(dir string, port int) (string, error) {
	path := SocketPathFor(dir, port)
	if strings.IndexFunc(path, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf(
			"%w: socket directory %q contains whitespace. The path is passed to "+
				"the postmaster in a space-separated options string, where it "+
				"would be split apart. Choose a directory without spaces",
			ErrInvalidArgument, dir)
	}
	if length := len(path); length > SUNPathMax {
		return "", fmt.Errorf(
			"%w: the unix socket path would be %d bytes, past the kernel's "+
				"%d-byte sun_path limit:\n\n    %s\n\nThis is a hard kernel "+
				"limit, not a PostgreSQL setting, and it applies to the whole "+
				"path including PostgreSQL's '%s' file name. Point the cluster's "+
				"socket directory at a shorter parent -- '/dev/shm' or '/tmp' -- "+
				"with the SocketParent option",
			ErrSocketPathTooLong, length, SUNPathMax, path,
			fmt.Sprintf(socketFileTemplate, port))
	}
	return path, nil
}

// pickParent chooses a parent directory, explicitly or from the candidate list.
//
// An explicit choice is never second-guessed: if it is unusable, that is an
// error, not a reason to quietly use something else. Without one, the candidates
// are tried in their fixed order and the first usable one wins.
func pickParent(explicit string, candidates []string, port int, checkSocket bool, what string) (string, error) {
	if explicit != "" {
		if !isDir(explicit) {
			return "", fmt.Errorf("%w: the %s parent %s does not exist or is not a directory",
				ErrPostgresUnavailable, what, explicit)
		}
		if checkSocket {
			// Probe with the longest name makeTempDir can produce below it.
			if _, err := CheckSocketDir(filepath.Join(explicit, probeName), port); err != nil {
				return "", err
			}
		}
		return explicit, nil
	}

	var tried []string
	for _, candidate := range candidates {
		if !isDir(candidate) || !isWritableDir(candidate) {
			tried = append(tried, candidate+" (missing or not writable)")
			continue
		}
		if checkSocket {
			if _, err := CheckSocketDir(filepath.Join(candidate, probeName), port); err != nil {
				tried = append(tried, fmt.Sprintf("%s (%s)", candidate, reasonName(err)))
				continue
			}
		}
		return candidate, nil
	}

	fallback := os.TempDir()
	if isDir(fallback) && isWritableDir(fallback) {
		if checkSocket {
			if _, err := CheckSocketDir(filepath.Join(fallback, probeName), port); err != nil {
				return "", err
			}
		}
		return fallback, nil
	}
	tried = append(tried, fallback+" (missing or not writable)")
	return "", fmt.Errorf("%w: no usable %s directory. Tried: %s. Pass an explicit %s parent",
		ErrPostgresUnavailable, what, strings.Join(tried, "; "), what)
}

// reasonName is the short label a rejected candidate is listed under.
func reasonName(err error) string {
	switch {
	case errors.Is(err, ErrSocketPathTooLong):
		return "socket path too long"
	case errors.Is(err, ErrInvalidArgument):
		return "unusable socket directory"
	default:
		return err.Error()
	}
}

// ---------------------------------------------------------------------------
// Names and throwaway directories
// ---------------------------------------------------------------------------

// GenerateName returns a fresh, unique, always-acceptable database name.
func GenerateName() string {
	return "test_" + randomHex(8)
}

// checkName refuses any name outside the closed character set. The name is
// interpolated into SQL, so the guarantee has to come from the character set
// rather than from a quoting routine.
func checkName(name string) error {
	if !safeName.MatchString(name) {
		return fmt.Errorf(
			"%w: database name %q is not accepted. Names created or dropped "+
				"through this package must match %s -- the name is interpolated "+
				"into SQL, and a closed character set is a stronger guarantee "+
				"than a quoting routine",
			ErrInvalidArgument, name, safeName.String())
	}
	return nil
}

// makeTempDir creates a fresh 0700 directory under parent whose name is prefix
// plus exactly eight hex characters.
//
// os.MkdirTemp is not used because its random suffix has no bounded length, and
// the socket directory's name length is the one thing standing between a
// caller and an unexplained bind() failure at SUNPathMax.
func makeTempDir(parent, prefix string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 64; attempt++ {
		path := filepath.Join(parent, prefix+randomHex(4))
		err := os.Mkdir(path, 0o700)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

// randomHex returns 2*n hex characters from the cryptographic source. The
// source is used because it cannot be seeded (and therefore cannot be made to
// repeat) by a test that plays with math/rand's global state.
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand.Read never returns an error on any supported platform;
		// Go 1.24 made it panic internally instead. Keeping the branch honest
		// costs nothing and documents that no silent fallback exists.
		panic("pgcluster: the system random source failed: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
