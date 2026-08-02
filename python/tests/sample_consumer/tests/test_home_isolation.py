"""Writing user configuration must land in a throwaway home."""

from __future__ import annotations

import os
import pwd

import acme
from conftest import HOME_SEEN_BY_CONFTEST


def test_config_is_written_under_the_throwaway_home():
    path = acme.save_config('name = "acme"\n')
    assert path.read_text() == 'name = "acme"\n'
    assert str(path).startswith(os.environ["HOME"])


def test_the_throwaway_home_is_not_the_developer_home():
    real_home = pwd.getpwuid(os.getuid()).pw_dir
    assert os.environ["HOME"] != real_home
    assert not acme.config_path().is_relative_to(real_home)


def test_the_conftest_already_saw_the_throwaway_home():
    assert HOME_SEEN_BY_CONFTEST == os.environ["HOME"]


def test_preserved_go_caches_are_still_reachable():
    """The opted-in carve-out survives the HOME repoint."""
    for var in ("GOPATH", "GOCACHE"):
        assert var in os.environ
        assert not os.environ[var].startswith(os.environ["HOME"])
