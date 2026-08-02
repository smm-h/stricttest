"""Autouse cwd isolation and the ``repo_cwd`` escape marker."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

# Captured at import time -- before any test body, therefore before the autouse
# chdir has moved anything. This is the cwd the session was invoked from.
INVOCATION_CWD = Path.cwd()


def test_every_test_runs_in_its_own_tmp_path(tmp_path):
    assert Path.cwd() == tmp_path


def test_cwd_is_not_the_project_root():
    assert Path.cwd() != Path(__file__).resolve().parent.parent


def test_an_unanchored_git_command_cannot_see_the_project_repo():
    """The point of the isolation: no walking up into the development repo."""
    out = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True
    )
    assert out.returncode != 0
    assert "not a git repository" in out.stderr.lower()


@pytest.mark.repo_cwd
def test_repo_cwd_marker_opts_out(tmp_path):
    assert Path.cwd() != tmp_path
    assert Path.cwd() == INVOCATION_CWD


def test_marker_is_registered_so_strict_markers_passes(inner):
    inner.write(
        {
            "test_marked.py": (
                "import pytest\n"
                "from pathlib import Path\n"
                "\n"
                "@pytest.mark.repo_cwd\n"
                "def test_opted_out(tmp_path):\n"
                "    assert Path.cwd() != tmp_path\n"
                "\n"
                "def test_isolated(tmp_path):\n"
                "    assert Path.cwd() == tmp_path\n"
            )
        }
    )
    inner.run("-q", "--strict-markers").assert_outcomes(passed=2)


def test_fixtures_that_chdir_into_the_same_tmp_path_compose(inner):
    inner.write(
        {
            "test_compose.py": (
                "import pytest\n"
                "from pathlib import Path\n"
                "\n"
                "@pytest.fixture\n"
                "def workdir(tmp_path, monkeypatch):\n"
                "    monkeypatch.chdir(tmp_path)\n"
                "    return tmp_path\n"
                "\n"
                "def test_same_directory(workdir, tmp_path):\n"
                "    assert Path.cwd() == workdir == tmp_path\n"
            )
        }
    )
    inner.run("-q").assert_outcomes(passed=1)
