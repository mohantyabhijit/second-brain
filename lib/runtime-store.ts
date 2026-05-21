import { mkdir, readFile, writeFile } from "fs/promises";
import path from "path";
import type { PhaseOneResult } from "./types";

const runtimeDir = path.join(process.cwd(), "data", "runtime");
const latestPath = path.join(runtimeDir, "latest-phase1.json");

export async function saveLatestResult(result: PhaseOneResult) {
  await mkdir(runtimeDir, { recursive: true });
  await writeFile(latestPath, JSON.stringify(result, null, 2), "utf8");
}

export async function readLatestResult() {
  try {
    const raw = await readFile(latestPath, "utf8");
    return JSON.parse(raw) as PhaseOneResult;
  } catch {
    return null;
  }
}
