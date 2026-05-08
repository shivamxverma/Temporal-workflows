import { NextResponse } from "next/server";

const workflowApiBase = process.env.WORKFLOW_API_BASE ?? "http://localhost:8080";

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params;
  const response = await fetch(`${workflowApiBase}/api/v1/workflow-runs/${id}`, {
    cache: "no-store",
  });

  return NextResponse.json(await response.json(), { status: response.status });
}
