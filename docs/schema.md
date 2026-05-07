# Sequential Workflow Engine Schema

This schema is the first MVP cut for a Temporal-like workflow engine built in Go.
It assumes:

- workflows are sequential
- task definitions are reusable templates
- each workflow run creates one ordered set of task runs
- workflow state is recoverable from task state plus append-only events

## Tables

### `workflow_definitions`

Reusable workflow template.

Example:

- `code_submission_workflow`

Important columns:

- `name`, `version`: allow safe evolution of workflow templates
- `is_active`: lets us keep old versions while disabling new starts

### `task_definitions`

Ordered step definitions for a workflow template.

Example task list for `code_submission_workflow`:

1. `create_submission`
2. `execute_code`
3. `save_result`
4. `mark_complete`

Important columns:

- `step_order`: sequential ordering
- `task_kind`: coarse task category
- `handler_name`: Go handler to invoke for that task
- `retry_*`, `timeout_seconds`: execution policy
- `config`: per-task config without a schema migration

### `workflow_runs`

One actual execution of a workflow definition.

Example:

- submission `sub_101` starts workflow run `wf_001`

Important columns:

- `business_id`: application identifier like submission ID
- `status`: workflow lifecycle
- `current_step_order`: current pointer for scheduler/orchestrator
- `input_payload`, `output_payload`: request/response payloads

### `task_runs`

Execution state for each task within one workflow run.

Important columns:

- `status`: task lifecycle
- `attempt_count`: retry tracking
- `lease_owner`, `lease_expires_at`: reclaim work after worker crash
- `input_payload`, `output_payload`, `error_message`: execution result

For the sequential MVP, there is exactly one row per `(workflow_run_id, step_order)`.

### `workflow_events`

Append-only event history.

Example event types:

- `workflow_started`
- `task_scheduled`
- `task_started`
- `task_completed`
- `task_failed`
- `workflow_completed`

This table is useful for:

- debugging
- auditing
- rebuilding state when needed
- future event-driven features

## Status Model

Suggested workflow statuses:

- `pending`
- `running`
- `completed`
- `failed`
- `cancelled`

Suggested task statuses:

- `pending`
- `scheduled`
- `running`
- `completed`
- `failed`
- `cancelled`

## Execution Model

1. Insert a `workflow_run`.
2. Materialize task rows from `task_definitions` into `task_runs`.
3. Mark the first task `scheduled`.
4. Worker claims the task using lease fields.
5. Worker marks it `running`, then `completed` or `failed`.
6. Orchestrator advances `current_step_order` and schedules the next task.
7. Append `workflow_events` at every important transition.

## Why This Split Exists

- `workflow_definitions` / `task_definitions` are templates
- `workflow_runs` / `task_runs` are live execution state

That keeps one workflow blueprint reusable across many submissions.

## MVP Notes

This is a solid first schema for Go, but still intentionally small.
We are not modeling:

- DAG dependencies
- child workflows
- signals
- timers
- dynamic branching
- versioned event replay

Those can be added later without changing the core distinction between definitions, runs, and events.
