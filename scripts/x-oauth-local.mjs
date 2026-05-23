#!/usr/bin/env node
import crypto from "node:crypto";
import http from "node:http";
import https from "node:https";
import { execFileSync, spawnSync } from "node:child_process";

const clientId = requiredEnv("X_CLIENT_ID");
const clientSecret = requiredEnv("X_CLIENT_SECRET");
const redirectUri = process.env.X_REDIRECT_URI || "http://127.0.0.1:8765/callback";
const scopes = (process.env.X_OAUTH_SCOPES || "tweet.read users.read bookmark.read offline.access").trim();
const oneCLI = process.env.ONECLI || "/Users/abhijitmohanty/.local/bin/onecli";
const oneCLIProject = process.env.ONECLI_PROJECT || "second-brain";
const user = process.env.USER || "abhijitmohanty";
const parsedRedirect = new URL(redirectUri);

if (parsedRedirect.hostname !== "127.0.0.1" && parsedRedirect.hostname !== "localhost") {
  throw new Error(`X_REDIRECT_URI must point to localhost for this helper, got ${redirectUri}`);
}

const codeVerifier = base64url(crypto.randomBytes(64));
const codeChallenge = base64url(crypto.createHash("sha256").update(codeVerifier).digest());
const state = base64url(crypto.randomBytes(24));

const authorizeURL = new URL("https://x.com/i/oauth2/authorize");
authorizeURL.searchParams.set("response_type", "code");
authorizeURL.searchParams.set("client_id", clientId);
authorizeURL.searchParams.set("redirect_uri", redirectUri);
authorizeURL.searchParams.set("scope", scopes);
authorizeURL.searchParams.set("state", state);
authorizeURL.searchParams.set("code_challenge", codeChallenge);
authorizeURL.searchParams.set("code_challenge_method", "S256");

const server = http.createServer(async (req, res) => {
  try {
    const incoming = new URL(req.url || "/", redirectUri);
    if (incoming.pathname !== parsedRedirect.pathname) {
      res.writeHead(404, { "Content-Type": "text/plain" });
      res.end("Not found");
      return;
    }
    if (incoming.searchParams.get("state") !== state) {
      throw new Error("OAuth state mismatch.");
    }
    const code = incoming.searchParams.get("code");
    if (!code) {
      throw new Error(incoming.searchParams.get("error_description") || incoming.searchParams.get("error") || "Missing OAuth code.");
    }
    const token = await exchangeCode(code);
    if (!token.access_token || !token.refresh_token) {
      throw new Error("X token response did not include both access_token and refresh_token.");
    }
    saveKeychain("second-brain/X_USER_ACCESS_TOKEN", token.access_token);
    saveKeychain("second-brain/X_REFRESH_TOKEN", token.refresh_token);
    updateOneCLISecret("Second Brain X user access token", token.access_token);
    updateOneCLISecret("Second Brain X refresh token", token.refresh_token);
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end("<h1>X authorization saved</h1><p>You can close this tab and run <code>npm run refresh:run</code>.</p>");
    console.log("Saved fresh X access and refresh tokens to Keychain.");
    console.log("If OneCLI secret update was available, the matching OneCLI token secrets were updated too.");
    server.close();
  } catch (error) {
    res.writeHead(500, { "Content-Type": "text/plain" });
    res.end(error instanceof Error ? error.message : String(error));
    console.error(error instanceof Error ? error.message : error);
    server.close(() => process.exitCode = 1);
  }
});

server.listen(Number(parsedRedirect.port || "80"), parsedRedirect.hostname, () => {
  console.log("Open this URL and approve X access:");
  console.log(authorizeURL.toString());
  console.log("");
  console.log(`Waiting for callback on ${redirectUri}`);
  if (process.env.X_OAUTH_OPEN_BROWSER !== "false") {
    spawnSync("open", [authorizeURL.toString()], { stdio: "ignore" });
  }
});

function exchangeCode(code) {
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: redirectUri,
    code_verifier: codeVerifier,
    client_id: clientId
  }).toString();
  const auth = Buffer.from(`${clientId}:${clientSecret}`).toString("base64");
  return postForm("https://api.x.com/2/oauth2/token", body, {
    Authorization: `Basic ${auth}`,
    "Content-Type": "application/x-www-form-urlencoded"
  });
}

function postForm(url, body, headers) {
  return new Promise((resolve, reject) => {
    const target = new URL(url);
    const req = https.request({
      method: "POST",
      hostname: target.hostname,
      path: `${target.pathname}${target.search}`,
      headers: {
        ...headers,
        "Content-Length": Buffer.byteLength(body)
      }
    }, (res) => {
      let raw = "";
      res.setEncoding("utf8");
      res.on("data", (chunk) => raw += chunk);
      res.on("end", () => {
        if (res.statusCode < 200 || res.statusCode >= 300) {
          reject(new Error(`X token exchange failed: ${res.statusCode} ${raw}`));
          return;
        }
        try {
          resolve(JSON.parse(raw));
        } catch (error) {
          reject(error);
        }
      });
    });
    req.on("error", reject);
    req.end(body);
  });
}

function saveKeychain(service, value) {
  execFileSync("security", ["add-generic-password", "-U", "-a", user, "-s", service, "-w", value], { stdio: "ignore" });
}

function updateOneCLISecret(name, value) {
  const list = spawnSync(oneCLI, ["secrets", "list", "--project", oneCLIProject], { encoding: "utf8" });
  if (list.status !== 0) {
    console.warn(`skip OneCLI update for ${name}: onecli secrets list failed`);
    return;
  }
  let payload;
  try {
    payload = JSON.parse(list.stdout);
  } catch {
    console.warn(`skip OneCLI update for ${name}: could not parse onecli output`);
    return;
  }
  const secret = payload.data?.find((item) => item.name === name);
  if (!secret?.id) {
    console.warn(`skip OneCLI update for ${name}: existing secret was not found`);
    return;
  }
  const update = spawnSync(oneCLI, ["secrets", "update", "--id", secret.id, "--value", value], { encoding: "utf8" });
  if (update.status !== 0) {
    console.warn(`skip OneCLI update for ${name}: ${update.stderr || update.stdout}`);
  }
}

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required. Store it in Keychain or export it before running this helper.`);
  }
  return value;
}

function base64url(buffer) {
  return Buffer.from(buffer).toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}
