create extension if not exists pgcrypto;

create table workflow_definitions (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  version integer not null default 1,
  description text,
  is_active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (name, version)
);

create table task_definitions (
  id uuid primary key default gen_random_uuid(),
  workflow_definition_id uuid not null references workflow_definitions(id) on delete cascade,
  name text not null,
  step_order integer not null,
  task_kind text not null check (task_kind in ('system', 'executor', 'persistence', 'notification')),
  handler_name text not null,
  retry_max_attempts integer not null default 3 check (retry_max_attempts >= 0),
  retry_backoff_seconds integer not null default 30 check (retry_backoff_seconds >= 0),
  timeout_seconds integer check (timeout_seconds is null or timeout_seconds > 0),
  config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (workflow_definition_id, name),
  unique (workflow_definition_id, step_order)
);

create table workflow_runs (
  id uuid primary key default gen_random_uuid(),
  workflow_definition_id uuid not null references workflow_definitions(id),
  business_id text,
  status text not null check (status in ('pending', 'running', 'completed', 'failed', 'cancelled')),
  current_step_order integer,
  input_payload jsonb not null default '{}'::jsonb,
  output_payload jsonb,
  error_message text,
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table task_runs (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  task_definition_id uuid not null references task_definitions(id),
  step_order integer not null,
  status text not null check (status in ('pending', 'scheduled', 'running', 'completed', 'failed', 'cancelled')),
  attempt_count integer not null default 0 check (attempt_count >= 0),
  input_payload jsonb not null default '{}'::jsonb,
  output_payload jsonb,
  error_message text,
  scheduled_at timestamptz,
  started_at timestamptz,
  completed_at timestamptz,
  lease_owner text,
  lease_expires_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (workflow_run_id, step_order)
);

create table workflow_events (
  id uuid primary key default gen_random_uuid(),
  workflow_run_id uuid not null references workflow_runs(id) on delete cascade,
  task_run_id uuid references task_runs(id) on delete set null,
  sequence_number bigint not null,
  event_type text not null,
  payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique (workflow_run_id, sequence_number)
);

create index idx_workflow_definitions_active
  on workflow_definitions (is_active);

create index idx_task_definitions_workflow_definition_id
  on task_definitions (workflow_definition_id, step_order);

create index idx_workflow_runs_status
  on workflow_runs (status);

create index idx_workflow_runs_business_id
  on workflow_runs (business_id);

create index idx_task_runs_workflow_run_id
  on task_runs (workflow_run_id, step_order);

create index idx_task_runs_status
  on task_runs (status);

create index idx_task_runs_lease_expires_at
  on task_runs (lease_expires_at)
  where status in ('scheduled', 'running');

create index idx_workflow_events_workflow_run_id
  on workflow_events (workflow_run_id, sequence_number);
