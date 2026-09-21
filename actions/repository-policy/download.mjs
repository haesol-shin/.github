import fs from "node:fs";
import https from "node:https";

const [uri, sizeText, destination] = process.argv.slice(2);
const limit = Number(sizeText);
if (!uri || !destination || !Number.isSafeInteger(limit) || limit <= 0) {
  throw new Error("artifact download arguments are invalid");
}

const deadline = Date.now() + 30_000;
let output;

function fail(message) {
  throw new Error(message);
}

async function download(urlText, redirects = 0) {
  if (redirects > 5) fail("artifact download exceeded redirect limit");
  const url = new URL(urlText);
  if (url.protocol !== "https:") fail("artifact URI must use HTTPS");

  await new Promise((resolve, reject) => {
    const remaining = deadline - Date.now();
    if (remaining <= 0) {
      reject(new Error("artifact download timed out"));
      return;
    }

    const request = https.get(url, { headers: { "User-Agent": "repo-ops-validator-action" } }, (response) => {
      const status = response.statusCode ?? 0;
      if (status >= 300 && status < 400 && response.headers.location) {
        response.resume();
        resolve(download(new URL(response.headers.location, url).href, redirects + 1));
        return;
      }
      if (status < 200 || status >= 300) {
        response.resume();
        reject(new Error(`artifact download returned HTTP ${status}`));
        return;
      }

      const declared = response.headers["content-length"];
      if (declared !== undefined && (!/^\d+$/.test(declared) || Number(declared) > limit)) {
        response.resume();
        reject(new Error("artifact response exceeds pinned size"));
        return;
      }

      let written = 0;
      output = fs.openSync(destination, "wx", 0o500);
      response.on("data", (chunk) => {
        if (written + chunk.length > limit) {
          response.destroy(new Error("artifact response exceeds pinned size"));
          return;
        }
        fs.writeSync(output, chunk);
        written += chunk.length;
      });
      response.on("end", () => {
        if (written !== limit) {
          reject(new Error(`artifact length mismatch: expected ${limit}, received ${written}`));
          return;
        }
        resolve();
      });
      response.on("error", reject);
    });

    const timer = setTimeout(
      () => request.destroy(new Error("artifact download timed out")),
      remaining,
    );
    request.on("close", () => clearTimeout(timer));
    request.on("error", reject);
  });
}

try {
  await download(uri);
} catch (error) {
  if (output !== undefined) fs.closeSync(output);
  fs.rmSync(destination, { force: true });
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}

if (output !== undefined) fs.closeSync(output);
