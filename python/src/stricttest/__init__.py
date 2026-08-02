"""stricttest -- an always-on test-isolation floor for pytest suites.

The package ships a ``pytest11`` plugin. Installing it is adoption: the plugin
loads automatically and refuses to run a suite that has not declared its safety
stance. See :mod:`stricttest.config` for the ini keys.
"""

from .config import PRESERVE_VARS, REQUIRED_KEYS, Settings
from .socketguard import NetworkBlocked, Policy

__version__ = "0.1.0"

__all__ = [
    "PRESERVE_VARS",
    "REQUIRED_KEYS",
    "NetworkBlocked",
    "Policy",
    "Settings",
    "__version__",
]
