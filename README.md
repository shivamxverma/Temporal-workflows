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

### `GET /healthz`

Basic health endpoint.

## Run

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/workflow_engine?sslmode=disable"
export HTTP_ADDR=":8080"
go run ./cmd/server
```

Apply the schema first from [db/schema.sql](/Users/shivamverma/Desktop/personal-work/workflow-engine-mvp/db/schema.sql).
# Temporal-workflows
