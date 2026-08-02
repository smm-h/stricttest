# stricttest (Go)

Test-environment hygiene helpers for Go suites: a throwaway HOME, an isolated
git config and identity, transport lockdown, credential stripping, and
cleanup-restoring chdir.

```bash
go get github.com/smm-h/stricttest/go
```

```go
import "github.com/smm-h/stricttest/go/hygiene"

func TestSomething(t *testing.T) {
	hygiene.Isolate(t)
	// HOME is a throwaway directory, git reads an empty global config under a
	// throwaway identity, only file:// transports are allowed, and every
	// ambient credential variable is gone -- all restored when the test ends.
}
```

## Surface

| Helper | What it binds |
|--------|---------------|
| `Isolate(t, opts...)` | Everything below, in one call |
| `ThrowawayHome(t) string` | `HOME` / `USERPROFILE` → a temporary directory (memoized per test) |
| `IsolateGitConfig(t)` | Empty `GIT_CONFIG_GLOBAL` / `GIT_CONFIG_SYSTEM`, throwaway author and committer identity, `GIT_TERMINAL_PROMPT=0` |
| `LockdownTransports(t)` | `GIT_ALLOW_PROTOCOL=file` |
| `StripCredentials(t)` | Removes every variable in `CredentialVars` |
| `Chdir(t, dir)` | Working directory for the test, restored afterwards (a failed restore fails the test) |
| `Preserve(v ...KnownVar)` | `Isolate` option: keeps the named toolchain caches pointing at the real home |

Every helper takes a `testing.TB`, undoes itself through `TB.Cleanup`, and
mutates the environment through `TB.Setenv` -- which panics when the test has
called `t.Parallel`. That is intended: a parallel test cannot own a
process-wide variable like `HOME`.

There is no network guard here. Go has no `sys.addaudithook` equivalent, so
network isolation for Go suites belongs to the sandbox runner, not to this
module.

## License

MIT
