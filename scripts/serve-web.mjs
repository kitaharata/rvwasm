#!/usr/bin/env node

import { createServer } from "node:http";
import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import process from "node:process";

const port = Number.parseInt(process.argv[2] ?? process.env.PORT ?? "8080", 10);
const host = process.argv[3] ?? process.env.HOST ?? "127.0.0.1";
const webDirArg = process.argv[4] ?? process.env.WEB_DIR ?? "../web/";
const webBaseUrl = new URL(webDirArg.endsWith("/") ? webDirArg : `${webDirArg}/`, import.meta.url);
const textHeaders = { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store" };

const files = new Map([
  ["/", { url: new URL("index.html", webBaseUrl), type: "text/html; charset=utf-8" }],
  ["/index.html", { url: new URL("index.html", webBaseUrl), type: "text/html; charset=utf-8" }],
  ["/riscv.wasm", { url: new URL("riscv.wasm", webBaseUrl), type: "application/wasm" }],
  ["/wasm_exec.js", { url: new URL("wasm_exec.js", webBaseUrl), type: "text/javascript; charset=utf-8" }],
]);

const server = createServer(async (req, res) => {
  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405, { ...textHeaders, "Allow": "GET, HEAD" });
    res.end("method not allowed\n");
    return;
  }

  const reqUrl = new URL(req.url ?? "/", "http://localhost");
  const entry = files.get(reqUrl.pathname);
  if (!entry) {
    res.writeHead(404, textHeaders);
    res.end("not found\n");
    return;
  }

  try {
    const st = await stat(entry.url);
    res.writeHead(200, {
      "Content-Type": entry.type,
      "Content-Length": st.size,
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    });
    if (req.method === "HEAD") {
      res.end();
      return;
    }
    createReadStream(entry.url).pipe(res);
  } catch {
    res.writeHead(404, textHeaders);
    res.end(`missing ${entry.url.pathname.split("/").pop()}\n`);
  }
});

server.listen(port, host, () => {
  console.log(`[rvwasm] serving ${webBaseUrl.href}`);
  console.log(`[rvwasm] listening on http://${host}:${port}/`);
});
