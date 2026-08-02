package hygiene

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitOrSkip returns the path to a real git binary, or skips the test with a
// precise reason. The meta-tests below are the only proof that a real tool
// honors the floor; asserting on environment variables alone would only prove
// the package talks to itself.
func gitOrSkip(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git is not on PATH (%v); the real-tool meta-test cannot run", err)
	}
	return path
}

func runGit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitOrSkip(t), args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ---------------------------------------------------------------------------
// ThrowawayHome
// ---------------------------------------------------------------------------

func TestThrowawayHomeRepointsHomeAndRestoresIt(t *testing.T) {
	t.Setenv("HOME", "/poisoned/home")
	t.Setenv("USERPROFILE", "/poisoned/home")

	var home string
	t.Run("during", func(t *testing.T) {
		home = ThrowawayHome(t)
		if home == "/poisoned/home" || home == "" {
			t.Fatalf("ThrowawayHome returned %q", home)
		}
		if got := os.Getenv("HOME"); got != home {
			t.Errorf("HOME = %q, want the throwaway %q", got, home)
		}
		if got := os.Getenv("USERPROFILE"); got != home {
			t.Errorf("USERPROFILE = %q, want the throwaway %q", got, home)
		}
		info, err := os.Stat(home)
		if err != nil || !info.IsDir() {
			t.Fatalf("the throwaway home is not a directory: %v", err)
		}
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatalf("reading the throwaway home: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("the throwaway home is not empty: %v", entries)
		}
	})

	if got := os.Getenv("HOME"); got != "/poisoned/home" {
		t.Errorf("HOME = %q after the subtest, want the poisoned value back", got)
	}
	if got := os.Getenv("USERPROFILE"); got != "/poisoned/home" {
		t.Errorf("USERPROFILE = %q after the subtest, want the poisoned value back", got)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("the throwaway home %q outlived its test (stat error: %v)", home, err)
	}
}

func TestThrowawayHomeIsWhatGoItselfReportsAsTheHomeDirectory(t *testing.T) {
	t.Run("during", func(t *testing.T) {
		home := ThrowawayHome(t)
		reported, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("os.UserHomeDir: %v", err)
		}
		if reported != home {
			t.Errorf("os.UserHomeDir() = %q, want the throwaway %q", reported, home)
		}
	})
}

func TestThrowawayHomeIsMemoizedWithinOneTest(t *testing.T) {
	t.Run("during", func(t *testing.T) {
		first := ThrowawayHome(t)
		second := ThrowawayHome(t)
		if first != second {
			t.Errorf("a second ThrowawayHome call moved HOME again: %q then %q", first, second)
		}
	})
}

func TestThrowawayHomeIsPerTest(t *testing.T) {
	var first, second string
	t.Run("a", func(t *testing.T) { first = ThrowawayHome(t) })
	t.Run("b", func(t *testing.T) { second = ThrowawayHome(t) })
	if first == second {
		t.Errorf("two subtests shared the throwaway home %q", first)
	}
}

// ---------------------------------------------------------------------------
// IsolateGitConfig
// ---------------------------------------------------------------------------

func TestIsolateGitConfigPointsAtEmptyFilesInTheThrowawayHome(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/poisoned/gitconfig")
	t.Setenv("GIT_CONFIG_SYSTEM", "/poisoned/gitconfig")

	t.Run("during", func(t *testing.T) {
		IsolateGitConfig(t)
		home := ThrowawayHome(t)
		for _, env := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM"} {
			path := os.Getenv(env)
			if !strings.HasPrefix(path, home+string(filepath.Separator)) {
				t.Errorf("%s = %q, want a file inside the throwaway home %q", env, path, home)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("%s points at %q which does not exist: %v", env, path, err)
			}
			if info.Size() != 0 {
				t.Errorf("%s points at %q which is not empty (%d bytes)", env, path, info.Size())
			}
		}
		if a, b := os.Getenv("GIT_CONFIG_GLOBAL"), os.Getenv("GIT_CONFIG_SYSTEM"); a == b {
			t.Errorf("the global and system config point at the same file %q", a)
		}
	})

	if got := os.Getenv("GIT_CONFIG_GLOBAL"); got != "/poisoned/gitconfig" {
		t.Errorf("GIT_CONFIG_GLOBAL = %q after the subtest, want the poisoned value back", got)
	}
}

func TestIsolateGitConfigSetsAThrowawayIdentity(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Real Developer")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")

	t.Run("during", func(t *testing.T) {
		IsolateGitConfig(t)
		for env, want := range map[string]string{
			"GIT_AUTHOR_NAME":     identityName,
			"GIT_AUTHOR_EMAIL":    identityEmail,
			"GIT_COMMITTER_NAME":  identityName,
			"GIT_COMMITTER_EMAIL": identityEmail,
			"GIT_TERMINAL_PROMPT": "0",
		} {
			if got := os.Getenv(env); got != want {
				t.Errorf("%s = %q, want %q", env, got, want)
			}
		}
	})

	if got := os.Getenv("GIT_AUTHOR_NAME"); got != "Real Developer" {
		t.Errorf("GIT_AUTHOR_NAME = %q after the subtest, want the poisoned value back", got)
	}
}

func TestRealGitSeesNoUserConfigAndTheThrowawayIdentity(t *testing.T) {
	gitOrSkip(t)
	t.Run("during", func(t *testing.T) {
		IsolateGitConfig(t)

		out, err := runGit(t, "config", "--global", "--list")
		if err == nil && strings.TrimSpace(out) != "" {
			t.Errorf("git read a non-empty global config:\n%s", out)
		}

		ident, err := runGit(t, "var", "GIT_AUTHOR_IDENT")
		if err != nil {
			t.Fatalf("git var GIT_AUTHOR_IDENT failed: %v\n%s", err, ident)
		}
		if !strings.Contains(ident, identityEmail) {
			t.Errorf("git author identity is %q, want the throwaway %q", strings.TrimSpace(ident), identityEmail)
		}
	})
}

// ---------------------------------------------------------------------------
// LockdownTransports
// ---------------------------------------------------------------------------

func TestLockdownTransportsAllowsOnlyFile(t *testing.T) {
	t.Setenv("GIT_ALLOW_PROTOCOL", "")
	t.Run("during", func(t *testing.T) {
		LockdownTransports(t)
		if got := os.Getenv("GIT_ALLOW_PROTOCOL"); got != "file" {
			t.Errorf("GIT_ALLOW_PROTOCOL = %q, want %q", got, "file")
		}
	})
	if got := os.Getenv("GIT_ALLOW_PROTOCOL"); got != "" {
		t.Errorf("GIT_ALLOW_PROTOCOL = %q after the subtest, want the previous value back", got)
	}
}

func TestRealGitRefusesANonFileTransport(t *testing.T) {
	gitOrSkip(t)
	t.Run("during", func(t *testing.T) {
		Isolate(t)
		// The protocol check happens before any connection is attempted, so
		// this never reaches the network even though the URL looks remote.
		out, err := runGit(t, "ls-remote", "ssh://example.invalid/repo.git")
		if err == nil {
			t.Fatalf("git accepted an ssh remote under the transport lockdown:\n%s", out)
		}
		if !strings.Contains(out, "transport 'ssh' not allowed") {
			t.Errorf("git failed for the wrong reason:\n%s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// StripCredentials
// ---------------------------------------------------------------------------

func TestStripCredentialsRemovesEveryVariableAndRestoresThem(t *testing.T) {
	for _, name := range CredentialVars {
		t.Setenv(name, "poisoned-"+name)
	}

	t.Run("during", func(t *testing.T) {
		StripCredentials(t)
		for _, name := range CredentialVars {
			if value, ok := os.LookupEnv(name); ok {
				t.Errorf("%s survived StripCredentials with value %q", name, value)
			}
		}
	})

	for _, name := range CredentialVars {
		if got := os.Getenv(name); got != "poisoned-"+name {
			t.Errorf("%s = %q after the subtest, want the poisoned value back", name, got)
		}
	}
}

func TestCredentialVarsCoversTheDocumentedVectors(t *testing.T) {
	present := make(map[string]bool, len(CredentialVars))
	for _, name := range CredentialVars {
		if present[name] {
			t.Errorf("%s is listed twice in CredentialVars", name)
		}
		present[name] = true
	}
	for _, name := range []string{
		"GH_TOKEN", "GITHUB_TOKEN", "GIT_ASKPASS", "SSH_AUTH_SOCK",
		"NPM_TOKEN", "PYPI_TOKEN",
	} {
		if !present[name] {
			t.Errorf("%s is missing from CredentialVars", name)
		}
	}
}

func TestUnsetEnvLeavesAnUnsetVariableUnset(t *testing.T) {
	const name = "STRICTTEST_HYGIENE_NEVER_SET"
	t.Run("during", func(t *testing.T) {
		unsetEnv(t, name)
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set after unsetEnv", name)
		}
	})
	if _, ok := os.LookupEnv(name); ok {
		t.Errorf("%s leaked out of the subtest as a set-but-empty variable", name)
	}
}
