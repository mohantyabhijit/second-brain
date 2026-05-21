import { NextResponse } from "next/server";
import { runPhaseOne } from "@/lib/pipeline";
import { readLatestResult } from "@/lib/runtime-store";

export async function GET() {
  const latest = await readLatestResult();
  return NextResponse.json({ latest });
}

export async function POST() {
  const result = await runPhaseOne();
  return NextResponse.json(result);
}
