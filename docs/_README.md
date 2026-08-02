---
title: README.md
---
# stricttest

An always-on test-isolation floor. A test suite should be structurally unable to
reach the real developer identity, an ambient credential, the network, or the
development repository -- not merely disciplined about avoiding them.

| Component | Install | Location |
|-----------|---------|----------|
| **Pytest plugin** | `pip install stricttest` | [python/](python/) |
| **Go env-hygiene module** | `go get github.com/smm-h/stricttest/go` | [go/](go/) |

## What the floor does

Installing the pytest plugin IS adoption -- there is no opt-in switch. Once
installed, every session binds:

- **Env poisoning.** A throwaway `HOME`, `USERPROFILE` and XDG directory set,
  created before any conftest module is imported.
- **Throwaway git identity and config.** `GIT_CONFIG_GLOBAL` / `GIT_CONFIG_SYSTEM`
  point at a session-local file carrying `protocol.ssh.allow=never` and a
  throwaway commit identity; `GIT_AUTHOR_*` / `GIT_COMMITTER_*` are pinned to it.
- **Transport lockdown.** `GIT_ALLOW_PROTOCOL=file`; ssh, proxy, terminal prompt
  and askpass are wired to hard-fail.
- **Credential stripping.** GitHub, npm, PyPI, cargo, AWS, Cloudflare and model
  API tokens plus the SSH agent socket are removed from the environment.
- **Socket guard.** A `sys.addaudithook` guard that refuses network connects,
  with host:port and unix-socket-path allowlists. Default is network-off.
- **Push guard.** A real `git push` to a non-local remote fails the test at the
  `subprocess.Popen` boundary.
- **Cwd isolation.** Every test runs chdir-ed into its own `tmp_path`, so an
  unanchored git command cannot operate on the development repo. Opt out per
  test with `@pytest.mark.repo_cwd`.
- **TMPDIR refusal.** A `TMPDIR` or `--basetemp` inside the repository aborts the
  session, because fixture temp dirs inside a repo let unanchored git commands
  walk up and commit junk.
- **Bare-run refusal.** A run collecting more than the threshold outside the
  sandbox runner is refused; small targeted runs stay bare-runnable.

## Configuration

Everything lives in `[tool.pytest.ini_options]`. Five safety keys are
**required**; a missing one aborts at configure time. There are no defaults for
them, on purpose.

```toml
[tool.pytest.ini_options]
stricttest_sockets = "deny"              # "deny" | "allowlist"
stricttest_socket_allowlist = []         # ["example.com:443", "[::1]:5432"]
stricttest_unix_socket_allowlist = []    # ["/run/pg/.s.PGSQL.5432"]
stricttest_loopback = "deny"             # "deny" | "allow"
stricttest_sandbox_required = "false"    # "true" once a sandbox runner exists
```

Optional keys, with their defaults:

| Key | Default | Meaning |
|-----|---------|---------|
| `stricttest_threshold` | `50` | Bare-run refusal threshold |
| `stricttest_sandbox_env` | `STRICTTEST_SANDBOX` | Env var the sandbox runner sets to `1` |
| `stricttest_runner_command` | `scripts/test.sh` | Command named in the refusal message |
| `stricttest_tmp_prefix` | `stricttest-env-` | Prefix of the throwaway env directory |
| `stricttest_git_user_name` | `stricttest` | Throwaway commit identity |
| `stricttest_git_user_email` | `stricttest@example.invalid` | Throwaway commit identity |
| `stricttest_preserve` | *(empty)* | Toolchain caches preserved across the HOME repoint |

`stricttest_preserve` accepts only a closed enum of known-safe toolchain
variables -- `go_path`, `go_mod_cache`, `go_cache`, `python_user_base`,
`cargo_home`, `rustup_home`, `npm_cache`, `uv_cache`, `pip_cache`,
`gradle_user_home`. Arbitrary environment variable names are rejected so a
credential vector can never become preservable by typo.

## Scope

The socket guard sees connects made through Python's `socket` module. Network
performed by a spawned subprocess (git, gh, psql) is invisible to it;
whole-process network isolation is the sandbox runner's job. The guard is the
in-process floor beneath it, not a replacement for it.

## License

MIT
