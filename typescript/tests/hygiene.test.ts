/**
 * The closed preserve enum and the concurrency detection.
 */

import assert from "node:assert/strict";
import { test } from "node:test";
import { isolate, KNOWN_VARS, type KnownVar, preserveVars } from "../src/index.js";
import { fakeRegistry } from "./support.js";

test("every preserve entry names a plain variable and a {home}-anchored default", () => {
	for (const [name, entry] of Object.entries(KNOWN_VARS)) {
		assert.match(name, /^[a-z][A-Za-z]*$/, `${name} is camelCase`);
		assert.match(entry.env, /^[A-Za-z_][A-Za-z0-9_]*$/, `${entry.env} is a var name`);
		assert.match(
			entry.default,
			/^\{(home|gopath)\}\//,
			`${name}'s default is anchored at the real home or GOPATH`,
		);
	}
});

test("preserveVars pins a variable that was already set", () => {
	const fake = fakeRegistry();
	const original = process.env["GOCACHE"];
	process.env["GOCACHE"] = "/somewhere/real";
	try {
		preserveVars(fake.registry, ["goCache"]);
		assert.equal(process.env["GOCACHE"], "/somewhere/real");
		// The whole point: the value survives the HOME repoint that follows.
		isolate(fake.registry);
		assert.equal(process.env["GOCACHE"], "/somewhere/real");
	} finally {
		fake.release();
		if (original === undefined) {
			delete process.env["GOCACHE"];
		} else {
			process.env["GOCACHE"] = original;
		}
	}
});

test("preserveVars fills an unset variable from its default under the real home", () => {
	const fake = fakeRegistry();
	const original = process.env["CARGO_HOME"];
	delete process.env["CARGO_HOME"];
	const realHome = process.env["HOME"];
	assert.ok(realHome);
	try {
		preserveVars(fake.registry, ["cargoHome"]);
		assert.equal(process.env["CARGO_HOME"], `${realHome}/.cargo`);
	} finally {
		fake.release();
		if (original === undefined) {
			delete process.env["CARGO_HOME"];
		} else {
			process.env["CARGO_HOME"] = original;
		}
	}
});

test("preserveVars resolves {gopath} against the pinned GOPATH", () => {
	const fake = fakeRegistry();
	const originals = {
		GOPATH: process.env["GOPATH"],
		GOMODCACHE: process.env["GOMODCACHE"],
	};
	process.env["GOPATH"] = "/custom/gopath";
	delete process.env["GOMODCACHE"];
	try {
		preserveVars(fake.registry, ["goPath", "goModCache"]);
		assert.equal(process.env["GOMODCACHE"], "/custom/gopath/pkg/mod");
	} finally {
		fake.release();
		for (const [name, value] of Object.entries(originals)) {
			if (value === undefined) {
				delete process.env[name];
			} else {
				process.env[name] = value;
			}
		}
	}
});

test("preserveVars rejects a name outside the closed enum at runtime", () => {
	// TypeScript rejects this at compile time; plain JavaScript consumers get
	// the same closed enum only because of this runtime check.
	const fake = fakeRegistry();
	try {
		assert.throws(
			() => preserveVars(fake.registry, ["OPENAI_API_KEY" as KnownVar]),
			/only the closed enum/,
		);
	} finally {
		fake.release();
	}
});

test("preserveVars with an empty list touches nothing", () => {
	const fake = fakeRegistry();
	const before = { ...process.env };
	try {
		preserveVars(fake.registry, []);
		assert.deepEqual({ ...process.env }, before);
	} finally {
		fake.release();
	}
});

test("nested isolation is fine and unwinds last-in-first-out", () => {
	const outer = fakeRegistry();
	const inner = fakeRegistry();
	const realHome = process.env["HOME"];
	isolate(outer.registry);
	const outerHome = process.env["HOME"];
	isolate(inner.registry);
	const innerHome = process.env["HOME"];
	assert.notEqual(innerHome, outerHome);
	inner.release();
	assert.equal(process.env["HOME"], outerHome, "the outer isolation survives");
	outer.release();
	assert.equal(process.env["HOME"], realHome);
});

test("two overlapping isolations are refused rather than silently interleaved", () => {
	// Out-of-order release leaves the environment half-restored by definition
	// -- that IS the damage being detected -- so this test puts it back by hand
	// rather than leaking a deleted HOME into the rest of the file.
	const before = { ...process.env };
	try {
		const a = fakeRegistry();
		const b = fakeRegistry();
		isolate(a.registry);
		isolate(b.registry);
		// Releasing the OUTER one first proves the two overlapped: neither test's
		// environment was ever really its own.
		assert.throws(() => a.release(), /isolation while another test's isolation/);
		b.release();
	} finally {
		for (const name of Object.keys(process.env)) {
			if (!Object.hasOwn(before, name)) {
				delete process.env[name];
			}
		}
		Object.assign(process.env, before);
	}
});

test("a helper called after release is a hard error, not a silent no-op", () => {
	const fake = fakeRegistry();
	isolate(fake.registry);
	fake.release();
	assert.throws(() => isolate(fake.registry), /already released/);
});
