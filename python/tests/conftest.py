"""Test harness for the stricttest plugin's own suite.

Most floor pieces can only be proved end-to-end, from a real pytest session
that adopts the plugin. Those tests run an INNER pytest session in a
subprocess (``pytester.runpytest_subprocess``) against a generated consumer
project. A subprocess is mandatory, not a stylistic choice: the socket guard's
audit hook is permanent for the life of a process, so a differing stance can
only be exercised in a fresh interpreter.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

import pytest

pytest_plugins = ["pytester"]

# The sample consumer is a complete project of its own -- it must run as its own
# pytest session with its own rootdir and stance, never as part of this suite.
collect_ignore = ["sample_consumer"]

# The most restrictive stance -- what a new consumer starts from. Individual
# tests override single keys.
SAFE_DEFAULTS: dict[str, object] = {
    "stricttest_sockets": "deny",
    "stricttest_socket_allowlist": [],
    "stricttest_unix_socket_allowlist": [],
    "stricttest_loopback": "deny",
    "stricttest_sandbox_required": "false",
}


def _toml_value(value: object) -> str:
    if isinstance(value, list):
        return "[" + ", ".join(json.dumps(str(v)) for v in value) + "]"
    return json.dumps(str(value))


def render_pyproject(ini: dict[str, object]) -> str:
    lines = ["[tool.pytest.ini_options]"]
    for key, value in ini.items():
        lines.append(f"{key} = {_toml_value(value)}")
    return "\n".join(lines) + "\n"


@dataclass
class InnerProject:
    """A generated consumer project plus the ability to run pytest inside it."""

    pytester: pytest.Pytester
    _runs: int = field(default=0, init=False)

    def write(
        self,
        files: dict[str, str],
        ini: dict[str, object] | None = None,
        omit: tuple[str, ...] = (),
    ) -> None:
        merged = dict(SAFE_DEFAULTS)
        merged.update(ini or {})
        for key in omit:
            merged.pop(key, None)
        self.pytester.makepyprojecttoml(render_pyproject(merged))
        for name, source in files.items():
            path = self.pytester.path / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(source)

    def run(self, *args: str) -> pytest.RunResult:
        """Run pytest inside the generated project.

        ``pytester`` puts its own ``--basetemp`` INSIDE the inner project root,
        which stricttest's own TMPDIR refusal correctly rejects. A basetemp
        outside the project root is appended so it wins the last-flag-wins
        argument race -- which is exactly the remediation the refusal message
        asks a consumer for.
        """
        self._runs += 1
        basetemp = self.pytester.path.parent / f"inner-basetemp-{self._runs}"
        return self.pytester.runpytest_subprocess(f"--basetemp={basetemp}", *args)


@pytest.fixture
def inner(pytester: pytest.Pytester) -> InnerProject:
    return InnerProject(pytester)
