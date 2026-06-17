import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL(".", import.meta.url));
const port = Number(process.env.PORT || 5178);

const mimeTypes = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
};

const hopByHopHeaders = new Set([
  "connection",
  "content-length",
  "host",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

const server = createServer(async (req, res) => {
  try {
    if (req.url === "/__proxy" && req.method === "POST") {
      await proxyRequest(req, res);
      return;
    }
    await serveStatic(req, res);
  } catch (error) {
    writeJSON(res, 500, { error: String(error?.message || error) });
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`API tester ready at http://127.0.0.1:${port}`);
});

async function serveStatic(req, res) {
  const url = new URL(req.url || "/", `http://${req.headers.host || "127.0.0.1"}`);
  const pathname = url.pathname === "/" ? "/index.html" : url.pathname;
  const filePath = normalize(join(root, pathname));
  if (!filePath.startsWith(root)) {
    writeJSON(res, 403, { error: "forbidden" });
    return;
  }
  try {
    const data = await readFile(filePath);
    res.writeHead(200, {
      "Content-Type": mimeTypes[extname(filePath)] || "application/octet-stream",
      "Cache-Control": "no-store",
    });
    res.end(data);
  } catch {
    writeJSON(res, 404, { error: "not found" });
  }
}

async function proxyRequest(req, res) {
  const payload = await readRequestJSON(req);
  const target = new URL(payload.url);
  if (!["http:", "https:"].includes(target.protocol)) {
    writeJSON(res, 400, { error: "only http and https targets are supported" });
    return;
  }

  const headers = new Headers();
  for (const [key, value] of Object.entries(payload.headers || {})) {
    if (value == null || value === "" || hopByHopHeaders.has(key.toLowerCase())) {
      continue;
    }
    headers.set(key, String(value));
  }

  const init = {
    method: payload.method || "GET",
    headers,
  };
  if (!["GET", "HEAD"].includes(init.method.toUpperCase()) && payload.body != null) {
    init.body = String(payload.body);
  }

  const started = Date.now();
  const upstream = await fetch(target, init);
  const buffer = Buffer.from(await upstream.arrayBuffer());
  const contentType = upstream.headers.get("content-type") || "";
  const isText = contentType.startsWith("text/") ||
    contentType.includes("json") ||
    contentType.includes("xml") ||
    contentType.includes("yaml") ||
    contentType.includes("csv");

  const responseHeaders = {};
  upstream.headers.forEach((value, key) => {
    responseHeaders[key] = value;
  });

  writeJSON(res, 200, {
    status: upstream.status,
    statusText: upstream.statusText,
    elapsedMs: Date.now() - started,
    headers: responseHeaders,
    body: isText ? buffer.toString("utf8") : buffer.toString("base64"),
    bodyEncoding: isText ? "utf8" : "base64",
  });
}

async function readRequestJSON(req) {
  let raw = "";
  for await (const chunk of req) {
    raw += chunk;
    if (raw.length > 2 * 1024 * 1024) {
      throw new Error("proxy request body too large");
    }
  }
  return raw ? JSON.parse(raw) : {};
}

function writeJSON(res, status, payload) {
  res.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Cache-Control": "no-store",
  });
  res.end(JSON.stringify(payload));
}
