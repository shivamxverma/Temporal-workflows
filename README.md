# Workflow Engine MVP

Small Go service for defining sequential workflows and their ordered tasks.

## APIs

### `POST /api/v1/workflow-definitions`

Creates a workflow definition and all ordered task definitions in one transaction.

Example request:

```json
{
  "name": "code_submission_workflow",
  "version": 1,
  "description": "Process a code execution submission",
  "tasks": [
    {
      "name": "create_submission",
      "task_kind": "system",
      "handler_name": "submission.create",
      "retry_max_attempts": 1
    },
    {
      "name": "execute_code",
      "task_kind": "executor",
      "handler_name": "executor.run_code",
      "retry_max_attempts": 3,
      "retry_backoff_seconds": 15,
      "timeout_seconds": 120,
      "config": {
        "language": "golang"
      }
    },
    {
      "name": "save_result",
      "task_kind": "persistence",
      "handler_name": "results.save"
    }
  ]
}
```

### `GET /api/v1/workflow-definitions/{id}`

Returns one workflow definition with its ordered tasks.

### `POST /api/v1/workflow-definitions/{id}/task-definitions`

Creates one task definition for an existing workflow definition. The new task is appended at the next `step_order`.

Example request:

```json
{
  "name": "notify_completion",
  "task_kind": "notification",
  "handler_name": "notifications.workflow_complete",
  "retry_max_attempts": 5,
  "retry_backoff_seconds": 10,
  "timeout_seconds": 30,
  "config": {
    "channel": "email"
  }
}
```

### `POST /api/v1/workflow-runs`

Creates one concrete workflow run from an existing active workflow definition. The response includes the created workflow run and all ordered task runs.

Example request:

```json
{
  "workflow_definition_id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
  "business_id": "submission_123",
  "input_payload": {
    "language": "golang",
    "source_code": "package main"
  }
}
```

Example response shape:

```json
{
  "id": "1d7b3e9c-d3f4-4f38-8a8c-37dfef4fb4f3",
  "workflow_definition_id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
  "status": "running",
  "current_step_order": 1,
  "input_payload": {},
  "task_runs": [
    {
      "id": "55b59057-e07e-4bfc-8d0f-c2bb0f38a7e4",
      "workflow_run_id": "1d7b3e9c-d3f4-4f38-8a8c-37dfef4fb4f3",
      "task_definition_id": "889d5d68-f4b0-41e4-b12a-a5aa85cd6304",
      "step_order": 1,
      "status": "scheduled",
      "attempt_count": 0,
      "input_payload": {}
    }
  ]
}
```

### `GET /api/v1/workflow-runs/{id}`

Returns one workflow run with its ordered task runs.

### `GET /healthz`

Basic health endpoint.

## Run

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/workflow_engine?sslmode=disable"
export HTTP_ADDR=":8080"
go run ./cmd/server
```

Apply the schema first from [db/schema.sql](/Users/shivamverma/Desktop/personal-work/workflow-engine-mvp/db/schema.sql).

## Frontend

A small Next.js console is available in [web/package.json](/Users/shivamverma/Desktop/personal-work/workflow-engine-mvp/web/package.json:1).

Run it in a second terminal:

```bash
cd web
cp .env.example .env.local
npm install
npm run dev
```

By default the frontend proxies requests to `http://localhost:8080`, so keep the Go API running on that port.
# Temporal-workflows
