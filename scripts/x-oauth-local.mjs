#!/usr/bin/env node
import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import { execFileSync, spawnSync } from "node:child_process";
import path from "node:path";

const clientId = requiredEnv("X_CLIENT_ID");
const clientSecret = process.env.X_CLIENT_SECRET?.trim() || "";
const redirectUri = process.env.X_REDIRECT_URI || "http://127.0.0.1:8765/callback";
const scopes = (process.env.X_OAUTH_SCOPES || "tweet.read tweet.write users.read bookmark.read offline.access").trim();
const oneCLI = process.env.ONECLI || "/Users/abhijitmohanty/.local/bin/onecli";
const oneCLIProject = process.env.ONECLI_PROJECT || "second-brain";
const user = process.env.USER || "abhijitmohanty";
const tokenSuffix = process.env.X_OAUTH_TOKEN_SUFFIX?.trim() || "";
const updateOneCLI = process.env.X_OAUTH_UPDATE_ONECLI !== "false";
const expectedUsername = normalizeUsername(process.env.X_EXPECTED_USERNAME || "mohantyabhijit");
const tokenRotationPath = process.env.X_TOKEN_ROTATION_PATH || new URL("../data/runtime/x-token-rotation.json", import.meta.url).pathname;
const reauthorizeCommand = process.env.X_REAUTHORIZE_COMMAND || (tokenSuffix === "_PROD" ? "npm run x:oauth:prod" : "npm run x:oauth");
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
    const profile = await fetchProfile(token.access_token);
    if (expectedUsername && normalizeUsername(profile.username) !== expectedUsername) {
      throw new Error(`X authenticated profile mismatch: expected @${expectedUsername}, got @${normalizeUsername(profile.username)}.`);
    }
    saveKeychain(`second-brain/X_USER_ACCESS_TOKEN${tokenSuffix}`, token.access_token);
    saveKeychain(`second-brain/X_REFRESH_TOKEN${tokenSuffix}`, token.refresh_token);
    writeTokenRotationMetadata(token);
    if (updateOneCLI) {
      upsertOneCLISecret({
        name: "Second Brain X user access token",
        value: token.access_token,
        createArgs: [
          "--host-pattern", "api.x.com",
          "--path-pattern", "/2/users/*",
          "--header-name", "Authorization",
          "--value-format", "Bearer {value}"
        ],
        updateArgs: [
          "--host-pattern", "api.x.com",
          "--path-pattern", "/2/users/*",
          "--header-name", "Authorization",
          "--value-format", "Bearer {value}"
        ]
      });
      upsertOneCLISecret({
        name: "Second Brain X refresh token",
        value: token.refresh_token,
        createArgs: [
          "--host-pattern", "api.x.com",
          "--path-pattern", "/2/oauth2/token",
          "--param-name", "refresh_token",
          "--param-format", "{value}"
        ],
        updateArgs: [
          "--host-pattern", "api.x.com",
          "--path-pattern", "/2/oauth2/token",
          "--param-name", "refresh_token",
          "--param-format", "{value}"
        ]
      });
    }
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end("<h1>X authorization saved</h1><p>You can close this tab and run <code>npm run refresh:run</code>.</p>");
    console.log(`Authorized X profile @${profile.username}.`);
    console.log(`Saved fresh X access and refresh tokens to Keychain services ending in ${tokenSuffix || "(no suffix)"}.`);
    console.log(`Saved X token rotation metadata to ${tokenRotationPath}.`);
    if (updateOneCLI) {
      console.log("Saved or updated the matching OneCLI token secrets for production runs.");
    } else {
      console.log("Skipped OneCLI token updates for this local/dev OAuth run.");
    }
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
  const headers = { "Content-Type": "application/x-www-form-urlencoded" };
  if (clientSecret) {
    headers.Authorization = `Basic ${Buffer.from(`${clientId}:${clientSecret}`).toString("base64")}`;
  }
  return postForm("https://api.x.com/2/oauth2/token", body, headers);
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

function fetchProfile(accessToken) {
  return new Promise((resolve, reject) => {
    const req = https.request({
      method: "GET",
      hostname: "api.x.com",
      path: "/2/users/me?user.fields=username,name",
      headers: { Authorization: `Bearer ${accessToken}` }
    }, (res) => {
      let raw = "";
      res.setEncoding("utf8");
      res.on("data", (chunk) => raw += chunk);
      res.on("end", () => {
        if (res.statusCode < 200 || res.statusCode >= 300) {
          reject(new Error(`X profile check failed: ${res.statusCode} ${raw}`));
          return;
        }
        try {
          const payload = JSON.parse(raw);
          if (!payload.data?.id || !payload.data?.username) {
            reject(new Error("X profile check did not return an authenticated user."));
            return;
          }
          resolve(payload.data);
        } catch (error) {
          reject(error);
        }
      });
    });
    req.on("error", reject);
    req.end();
  });
}

function saveKeychain(service, value) {
  execFileSync("security", ["add-generic-password", "-U", "-a", user, "-s", service, "-w", value], { stdio: "ignore" });
}

function writeTokenRotationMetadata(token) {
  const rotatedAt = new Date();
  const expiresIn = Number(token.expires_in || 0);
  const accessTokenExpiresAt = expiresIn > 0 ? new Date(rotatedAt.getTime() + expiresIn * 1000) : null;
  const metadata = {
    rotatedAt: rotatedAt.toISOString(),
    ...(accessTokenExpiresAt ? { accessTokenExpiresAt: accessTokenExpiresAt.toISOString() } : {}),
    ...(expiresIn > 0 ? { expiresInSeconds: expiresIn } : {}),
    ...(token.scope ? { scope: String(token.scope) } : {}),
    ...(token.token_type ? { tokenType: String(token.token_type) } : {}),
    keychainTokenSuffix: tokenSuffix,
    onecliGateway: updateOneCLI,
    expectedUsername,
    reauthorizeCommand
  };
  fs.mkdirSync(path.dirname(tokenRotationPath), { recursive: true, mode: 0o700 });
  fs.writeFileSync(tokenRotationPath, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 });
}

function upsertOneCLISecret(definition) {
  const list = spawnSync(oneCLI, ["secrets", "list", "--project", oneCLIProject], { encoding: "utf8" });
  if (list.status !== 0) {
    console.warn(`skip OneCLI update for ${definition.name}: onecli secrets list failed`);
    return;
  }
  let payload;
  try {
    payload = JSON.parse(list.stdout);
  } catch {
    console.warn(`skip OneCLI update for ${definition.name}: could not parse onecli output`);
    return;
  }
  const secret = payload.data?.find((item) => item.name === definition.name);
  if (secret?.id) {
    const update = spawnSync(oneCLI, [
      "secrets", "update",
      "--id", secret.id,
      "--value", definition.value,
      ...definition.updateArgs
    ], { encoding: "utf8" });
    if (update.status !== 0) {
      console.warn(`skip OneCLI update for ${definition.name}: ${update.stderr || update.stdout}`);
    }
    return;
  }

  const create = spawnSync(oneCLI, [
    "secrets", "create",
    "--project", oneCLIProject,
    "--name", definition.name,
    "--type", "generic",
    "--value", definition.value,
    ...definition.createArgs
  ], { encoding: "utf8" });
  if (create.status !== 0) {
    console.warn(`skip OneCLI create for ${definition.name}: ${create.stderr || create.stdout}`);
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

function normalizeUsername(username) {
  return String(username || "").trim().replace(/^@/, "").toLowerCase();
}
