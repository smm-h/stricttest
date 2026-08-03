"""The two floors must strip and preserve the same variables.

The plugin and the Go ``hygiene`` package ship in one repository and are adopted
side by side in polyglot repos. If their lists drift, a repo gets one guarantee
in Python and a different one in Go -- silently. These tests read the Go source
directly, so drift is a failure here rather than a discovery in production.

They skip when the Go module is not on disk (an installed wheel carries only the
Python package).
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

from stricttest.config import PRESERVE_VARS
from stricttest.envfloor import CREDENTIAL_VARS

GO_ROOT = Path(__file__).resolve().parents[2] / "go" / "hygiene"

# GIT_ASKPASS is stripped by the Go floor and pinned to /bin/false by the Python
# one. Both floors otherwise lock transports down identically (GIT_ALLOW_PROTOCOL
# plus a pinned GIT_SSH_COMMAND and GIT_PROXY_COMMAND), and either treatment
# leaves git unable to obtain a credential, so this is the one deliberate
# difference between the lists.
GO_ONLY_CREDENTIAL_VARS = {"GIT_ASKPASS"}


def _go_source(name: str) -> str:
    path = GO_ROOT / name
    if not path.exists():
        pytest.skip(f"the Go module is not present at {GO_ROOT}")
    return path.read_text()


def _go_credential_vars() -> set[str]:
    source = _go_source("env.go")
    block = re.search(r"var CredentialVars = \[\]string\{(.*?)\n\}", source, re.DOTALL)
    assert block, "CredentialVars is no longer a []string literal in env.go"
    return set(re.findall(r'"([A-Za-z0-9_]+)"', block.group(1)))


def _go_preserve_vars() -> set[str]:
    source = _go_source("hygiene.go")
    block = re.search(
        r"var knownVars = map\[KnownVar\]knownVar\{(.*?)\n\}", source, re.DOTALL
    )
    assert block, "knownVars is no longer a map literal in hygiene.go"
    # Each entry is {"Name", "ENV_VAR", "{home}/default"}.
    return set(re.findall(r'\{"[A-Za-z]+",\s*"([A-Za-z0-9_]+)"', block.group(1)))


def test_the_credential_lists_match():
    assert _go_credential_vars() == set(CREDENTIAL_VARS) | GO_ONLY_CREDENTIAL_VARS


def test_the_preserve_enums_cover_the_same_variables():
    python_vars = {env for env, _ in PRESERVE_VARS.values()}
    assert _go_preserve_vars() == python_vars
