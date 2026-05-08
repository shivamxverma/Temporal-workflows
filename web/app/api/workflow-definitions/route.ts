import { NextRequest, NextResponse } from "next/server";

const workflowApiBase = process.env.WORKFLOW_API_BASE ?? "http://localhost:8080";

export async function POST(request: NextRequest) {
  const body = await request.text();
  const response = await fetch(`${workflowApiBase}/api/v1/workflow-definitions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    cache: "no-store",
  });

  return NextResponse.json(await response.json(), { status: response.status });
}
