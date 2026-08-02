package hygiene

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The throwaway commit identity. It is intentionally an invalid address: a
// commit made under it can never be mistaken for one of the developer's own,
// and mail to it goes nowhere.
const (
	identityName  = "stricttest"
	identityEmail = "stricttest@example.invalid"
)

// CredentialVars is the closed list of ambient credential vectors that
// [StripCredentials] removes from the environment. A test that genuinely needs
// one sets a FAKE value itself with TB.Setenv.
//
// The list mirrors the Python plugin's CREDENTIAL_VARS, plus GIT_ASKPASS: the
// Python floor pins GIT_ASKPASS to /bin/false as part of its wider transport
// lockdown, while this package's [LockdownTransports] is narrower, so the
// variable is stripped here instead.
var CredentialVars = []string{
	"SSH_AUTH_SOCK",
	"GIT_ASKPASS",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"GITHUB_API_TOKEN",
	"NPM_TOKEN",
	"NODE_AUTH_TOKEN",
	"PYPI_TOKEN",
	"TWINE_PASSWORD",
	"TWINE_USERNAME",
	"CARGO_REGISTRY_TOKEN",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"CLOUDFLARE_API_TOKEN",
	"CF_PAGES_API_TOKEN",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
}

var (
	homesMu sync.Mutex
	// homes memoizes the throwaway home per TB so that Isolate and a later
	// direct ThrowawayHome call (or IsolateGitConfig, which needs the home to
	// write into) all agree on one directory per test.
	homes = map[testing.TB]string{}
)

// ThrowawayHome repoints HOME and USERPROFILE at a fresh temporary directory
// owned by t, and returns that directory. Both variables are restored when t
// finishes, and the directory is removed with the rest of t's TempDir tree.
//
// The call is memoized per TB: asking twice within the same test returns the
// same directory rather than moving HOME again. Each subtest gets its own TB
// and therefore its own home.
func ThrowawayHome(t testing.TB) string {
	t.Helper()
	homesMu.Lock()
	existing, ok := homes[t]
	homesMu.Unlock()
	if ok {
		return existing
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	homesMu.Lock()
	homes[t] = home
	homesMu.Unlock()
	t.Cleanup(func() {
		homesMu.Lock()
		delete(homes, t)
		homesMu.Unlock()
	})
	return home
}

// IsolateGitConfig cuts git off from the developer's configuration and
// identity: GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM point at empty files inside
// the throwaway home (which it allocates through [ThrowawayHome] if the test
// has not already), the author and committer identity become a throwaway one,
// and GIT_TERMINAL_PROMPT is 0 so no git invocation can ever block a test on a
// password prompt.
//
// The config files are empty rather than carrying the identity, because the
// GIT_AUTHOR_* / GIT_COMMITTER_* variables below already supply it and outrank
// any config file -- a git invocation that ignores the config path entirely
// still cannot commit as the developer.
//
// core.hooksPath is deliberately NOT set. It overrides repo-local hooks too,
// which would silently disable a suite's own pre-push-hook tests; an empty
// global config already prevents the developer's hooks from firing.
func IsolateGitConfig(t testing.TB) {
	t.Helper()
	home := ThrowawayHome(t)

	for _, cfg := range []struct{ env, file string }{
		{"GIT_CONFIG_GLOBAL", "gitconfig-global"},
		{"GIT_CONFIG_SYSTEM", "gitconfig-system"},
	} {
		path := filepath.Join(home, cfg.file)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("hygiene: writing the throwaway %s file: %v", cfg.env, err)
			return
		}
		t.Setenv(cfg.env, path)
	}

	t.Setenv("GIT_AUTHOR_NAME", identityName)
	t.Setenv("GIT_AUTHOR_EMAIL", identityEmail)
	t.Setenv("GIT_COMMITTER_NAME", identityName)
	t.Setenv("GIT_COMMITTER_EMAIL", identityEmail)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

// LockdownTransports restricts git to the local file transport for the
// duration of t. Any ssh://, https:// or git:// URL a test reaches for -- a
// real remote, a real fetch, a real push -- fails at the protocol check instead
// of touching the network.
func LockdownTransports(t testing.TB) {
	t.Helper()
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
}

// StripCredentials removes every variable in [CredentialVars] from the
// environment for the duration of t. The original values are restored when t
// finishes.
func StripCredentials(t testing.TB) {
	t.Helper()
	for _, name := range CredentialVars {
		unsetEnv(t, name)
	}
}

// unsetEnv removes name from the environment until t finishes.
//
// TB has no Unsetenv, so this goes through TB.Setenv first and then unsets the
// (now empty) variable directly. Routing through TB.Setenv is what registers
// the restore -- and what keeps the T.Parallel panic intact, which a bare
// os.Unsetenv would silently skip.
func unsetEnv(t testing.TB, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("hygiene: unsetting %s: %v", name, err)
	}
}
