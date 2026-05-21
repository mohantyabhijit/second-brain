import { existsSync } from "fs";

export const ONECLI_BIN = "/Users/abhijitmohanty/.local/bin/onecli";

export function hasOneCli() {
  return existsSync(ONECLI_BIN);
}

export function getEnv(name: string) {
  const value = process.env[name];
  return value && value.trim() ? value.trim() : undefined;
}

export function oneCliGatewayEnabled() {
  return process.env.ONECLI_GATEWAY === "true";
}

export function credentialHint(name: string) {
  return `${name} is not present in process env. Store it in OneCLI and run the app through onecli run, or export it only for a local validation session.`;
}
