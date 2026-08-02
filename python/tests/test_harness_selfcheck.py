"""The harness itself must work before anything built on it means anything."""


def test_inner_project_runs_green(inner):
    inner.write({"test_ok.py": "def test_ok():\n    assert True\n"})
    result = inner.run("-q")
    result.assert_outcomes(passed=1)
