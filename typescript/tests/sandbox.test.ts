/**
 * Bare-run refusal.
 */

import assert from "node:assert/strict";
import { test } from "node:test";
import {
	DEFAULT_RUNNER_COMMAND,
	DEFAULT_SANDBOX_ENV,
	insideSandbox,
	requireSandbox,
	SandboxRequiredError,
} from "../src/index.js";

/** Run `fn` with the sandbox variable forced to `value` (or removed). */
function withSandboxEnv(value: string | undefined, fn: () => void): void {
	const original = Object.hasOwn(process.env, DEFAULT_SANDBOX_ENV)
		? process.env[DEFAULT_SANDBOX_ENV]
		: undefined;
	if (value === undefined) {
		delete process.env[DEFAULT_SANDBOX_ENV];
	} else {
		process.env[DEFAULT_SANDBOX_ENV] = value;
	}
	try {
		fn();
	} finally {
		if (original === undefined) {
			delete process.env[DEFAULT_SANDBOX_ENV];
		} else {
			process.env[DEFAULT_SANDBOX_ENV] = original;
		}
	}
}

test("insideSandbox is true only for exactly '1'", () => {
	withSandboxEnv("1", () => assert.equal(insideSandbox(), true));
	withSandboxEnv("0", () => assert.equal(insideSandbox(), false));
	withSandboxEnv("true", () => assert.equal(insideSandbox(), false));
	withSandboxEnv(undefined, () => assert.equal(insideSandbox(), false));
});

test("policy 'always' refuses every bare run", () => {
	withSandboxEnv(undefined, () => {
		assert.throws(
			() => requireSandbox({ policy: "always" }),
			(error: unknown) => {
				assert.ok(error instanceof SandboxRequiredError);
				assert.match(error.message, /Refusing to run this suite outside the sandbox/);
				assert.match(error.message, new RegExp(DEFAULT_RUNNER_COMMAND));
				assert.match(error.message, new RegExp(`${DEFAULT_SANDBOX_ENV}=1`));
				return true;
			},
		);
	});
});

test("policy 'always' permits a run inside the sandbox", () => {
	withSandboxEnv("1", () => {
		requireSandbox({ policy: "always" });
	});
});

test("policy 'threshold' refuses a run over the threshold", () => {
	withSandboxEnv(undefined, () => {
		assert.throws(
			() => requireSandbox({ policy: "threshold", threshold: 50, count: 51 }),
			(error: unknown) => {
				assert.ok(error instanceof SandboxRequiredError);
				assert.match(error.message, /Refusing to run 51 tests bare \(> 50\)/);
				return true;
			},
		);
	});
});

test("policy 'threshold' permits a run at or under the threshold", () => {
	withSandboxEnv(undefined, () => {
		requireSandbox({ policy: "threshold", threshold: 50, count: 50 });
		requireSandbox({ policy: "threshold", threshold: 50, count: 0 });
	});
});

test("policy 'threshold' permits any size inside the sandbox", () => {
	withSandboxEnv("1", () => {
		requireSandbox({ policy: "threshold", threshold: 1, count: 10_000 });
	});
});

test("the sandbox variable name and runner command are overridable", () => {
	const original = process.env["MY_SANDBOX"];
	delete process.env["MY_SANDBOX"];
	try {
		assert.throws(
			() =>
				requireSandbox({
					policy: "always",
					sandboxEnv: "MY_SANDBOX",
					runnerCommand: "make sandbox-test",
				}),
			/make sandbox-test[\s\S]*MY_SANDBOX=1/,
		);
		process.env["MY_SANDBOX"] = "1";
		requireSandbox({ policy: "always", sandboxEnv: "MY_SANDBOX" });
	} finally {
		if (original === undefined) {
			delete process.env["MY_SANDBOX"];
		} else {
			process.env["MY_SANDBOX"] = original;
		}
	}
});

test("a zero or fractional threshold is a hard error, not a silent clamp", () => {
	withSandboxEnv("1", () => {
		assert.throws(
			() => requireSandbox({ policy: "threshold", threshold: 0, count: 1 }),
			/threshold must be an integer >= 1/,
		);
		assert.throws(
			() => requireSandbox({ policy: "threshold", threshold: 1.5, count: 1 }),
			/threshold must be an integer >= 1/,
		);
	});
});

test("a negative or fractional count is a hard error", () => {
	withSandboxEnv("1", () => {
		assert.throws(
			() => requireSandbox({ policy: "threshold", threshold: 5, count: -1 }),
			/count must be a non-negative integer/,
		);
		assert.throws(
			() => requireSandbox({ policy: "threshold", threshold: 5, count: 2.5 }),
			/count must be a non-negative integer/,
		);
	});
});

test("an unknown policy is refused instead of defaulting to something", () => {
	withSandboxEnv("1", () => {
		assert.throws(
			() =>
				requireSandbox({ policy: "sometimes" } as unknown as Parameters<
					typeof requireSandbox
				>[0]),
			/needs an explicit policy/,
		);
	});
});

test("validation happens before the sandbox check, so a bad call never passes", () => {
	// A misconfigured guard must be loud even when the run happens to be inside
	// the sandbox -- otherwise the mistake only surfaces on the bare run it was
	// supposed to refuse.
	withSandboxEnv("1", () => {
		assert.throws(() =>
			requireSandbox({ policy: "threshold", threshold: 0, count: 1 }),
		);
	});
});
