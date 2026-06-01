#!/usr/bin/env node

import * as fs from "node:fs";
import * as os from "node:os";
import process from "node:process";
import { webcrypto } from "node:crypto";
import { performance } from "node:perf_hooks";
import { pathToFileURL } from "node:url";

const [wasmPath, fwPath, stepsArg = "20000000"] = process.argv.slice(2);
if (!wasmPath || !fwPath) {
  console.error("usage: node scripts/run-fw-payload.mjs <riscv.wasm> <fw_payload.bin> [steps]");
  process.exit(2);
}

const steps = Number.parseInt(stepsArg, 10);
if (!Number.isSafeInteger(steps) || steps <= 0 || String(steps) !== stepsArg) {
  console.error(`invalid steps: ${stepsArg}`);
  process.exit(2);
}

globalThis.fs = fs;
globalThis.process = process;
globalThis.crypto ??= webcrypto;
globalThis.performance ??= performance;
globalThis.appendTerminal = (s) => process.stdout.write(String(s));

const wasmUrl = new URL(wasmPath, pathToFileURL(`${process.cwd()}/`));
const wasmExecUrl = new URL("./wasm_exec.js", wasmUrl);
await import(wasmExecUrl);

const tmpDir = process.env.TMPDIR || os.tmpdir();
const go = new Go();
go.argv = [wasmPath, fwPath, stepsArg];
go.env = { ...process.env, TMPDIR: tmpDir };
go.exit = (code) => { process.exitCode = code; };

const wasmBytes = await fs.promises.readFile(wasmPath);
const response = new Response(wasmBytes, { headers: { "Content-Type": "application/wasm" } });
const { instance } = await WebAssembly.instantiateStreaming(response, go.importObject);

go.run(instance);
while (!globalThis.rvwasmReady) await new Promise(setImmediate);

const fwBytes = await fs.promises.readFile(fwPath);
const fw = new Uint8Array(fwBytes);

console.error(`[rvwasm] ${rvwasmLoadFirmware(fw)}`);
console.error(`[rvwasm] ${rvwasmStep(steps)}`);
console.error(`[rvwasm] ${rvwasmStatus()}`);

process.exit(process.exitCode ?? 0);
