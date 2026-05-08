"use client";

import { FormEvent, useState } from "react";

type TaskDefinitionForm = {
  name: string;
  task_kind: string;
  handler_name: string;
  retry_max_attempts: string;
  retry_backoff_seconds: string;
  timeout_seconds: string;
  config: string;
};

type WorkflowDefinitionResponse = {
  id: string;
  name: string;
  version: number;
  description: string;
  is_active: boolean;
  tasks: Array<{
    id: string;
    name: string;
    step_order: number;
    task_kind: string;
    handler_name: string;
  }>;
};

type WorkflowRunResponse = {
  id: string;
  workflow_definition_id: string;
  business_id?: string;
  status: string;
  current_step_order?: number;
  task_runs: Array<{
    id: string;
    workflow_run_id: string;
    task_definition_id: string;
    step_order: number;
    status: string;
    attempt_count: number;
  }>;
};

const defaultTask = (): TaskDefinitionForm => ({
  name: "",
  task_kind: "system",
  handler_name: "",
  retry_max_attempts: "",
  retry_backoff_seconds: "",
  timeout_seconds: "",
  config: "{}",
});

const taskKinds = ["system", "executor", "persistence", "notification"];

export default function HomePage() {
  const [workflowName, setWorkflowName] = useState("code_submission_workflow");
  const [workflowVersion, setWorkflowVersion] = useState("1");
  const [workflowDescription, setWorkflowDescription] = useState("Process a code execution submission");
  const [workflowActive, setWorkflowActive] = useState(true);
  const [tasks, setTasks] = useState<TaskDefinitionForm[]>([
    {
      name: "create_submission",
      task_kind: "system",
      handler_name: "submission.create",
      retry_max_attempts: "1",
      retry_backoff_seconds: "",
      timeout_seconds: "",
      config: "{}",
    },
    {
      name: "execute_code",
      task_kind: "executor",
      handler_name: "executor.run_code",
      retry_max_attempts: "3",
      retry_backoff_seconds: "15",
      timeout_seconds: "120",
      config: '{\n  "language": "golang"\n}',
    },
  ]);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState("");
  const [newTask, setNewTask] = useState<TaskDefinitionForm>(defaultTask());
  const [runWorkflowDefinitionId, setRunWorkflowDefinitionId] = useState("");
  const [businessId, setBusinessId] = useState("submission_123");
  const [inputPayload, setInputPayload] = useState('{\n  "language": "golang",\n  "submission_id": "submission_123"\n}');
  const [lookupRunId, setLookupRunId] = useState("");
  const [definitionResult, setDefinitionResult] = useState<WorkflowDefinitionResponse | null>(null);
  const [taskResult, setTaskResult] = useState<unknown>(null);
  const [runResult, setRunResult] = useState<WorkflowRunResponse | null>(null);
  const [lookupRunResult, setLookupRunResult] = useState<WorkflowRunResponse | null>(null);
  const [message, setMessage] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [loadingKey, setLoadingKey] = useState<string>("");

  async function readJsonResponse(response: Response) {
    const data = await response.json().catch(() => null);
    if (!response.ok) {
      const messageText =
        data && typeof data === "object" && "message" in data
          ? String((data as { message: unknown }).message)
          : `Request failed with status ${response.status}`;
      throw new Error(messageText);
    }

    return data;
  }

  function setFeedback(nextMessage: string, nextError = "") {
    setMessage(nextMessage);
    setError(nextError);
  }

  function parseJsonField(value: string, fieldName: string) {
    try {
      return JSON.parse(value);
    } catch {
      throw new Error(`${fieldName} must be valid JSON`);
    }
  }

  function normalizeTask(task: TaskDefinitionForm) {
    const payload: Record<string, unknown> = {
      name: task.name.trim(),
      task_kind: task.task_kind,
      handler_name: task.handler_name.trim(),
    };

    if (task.retry_max_attempts.trim() !== "") {
      payload.retry_max_attempts = Number(task.retry_max_attempts);
    }
    if (task.retry_backoff_seconds.trim() !== "") {
      payload.retry_backoff_seconds = Number(task.retry_backoff_seconds);
    }
    if (task.timeout_seconds.trim() !== "") {
      payload.timeout_seconds = Number(task.timeout_seconds);
    }

    payload.config = parseJsonField(task.config, `Config for ${task.name || "task"}`);
    return payload;
  }

  async function handleCreateWorkflowDefinition(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoadingKey("definition");
    setFeedback("Creating workflow definition...");

    try {
      const payload = {
        name: workflowName.trim(),
        version: Number(workflowVersion),
        description: workflowDescription.trim(),
        is_active: workflowActive,
        tasks: tasks.map(normalizeTask),
      };

      const response = await fetch("/api/workflow-definitions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });

      const data = (await readJsonResponse(response)) as WorkflowDefinitionResponse;
      setDefinitionResult(data);
      setSelectedWorkflowId(data.id);
      setRunWorkflowDefinitionId(data.id);
      setFeedback(`Workflow definition ${data.name} created.`);
    } catch (err) {
      setFeedback("", err instanceof Error ? err.message : "Failed to create workflow definition");
    } finally {
      setLoadingKey("");
    }
  }

  async function handleCreateTaskDefinition(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoadingKey("task");
    setFeedback("Appending task definition...");

    try {
      if (!selectedWorkflowId.trim()) {
        throw new Error("Workflow definition ID is required");
      }

      const response = await fetch(`/api/workflow-definitions/${selectedWorkflowId}/task-definitions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(normalizeTask(newTask)),
      });

      const data = await readJsonResponse(response);
      setTaskResult(data);
      setNewTask(defaultTask());
      setFeedback("Task definition appended.");
    } catch (err) {
      setFeedback("", err instanceof Error ? err.message : "Failed to create task definition");
    } finally {
      setLoadingKey("");
    }
  }

  async function handleCreateWorkflowRun(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoadingKey("run");
    setFeedback("Starting workflow run...");

    try {
      const response = await fetch("/api/workflow-runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workflow_definition_id: runWorkflowDefinitionId.trim(),
          business_id: businessId.trim(),
          input_payload: parseJsonField(inputPayload, "Input payload"),
        }),
      });

      const data = (await readJsonResponse(response)) as WorkflowRunResponse;
      setRunResult(data);
      setLookupRunId(data.id);
      setLookupRunResult(data);
      setFeedback(`Workflow run ${data.id} created.`);
    } catch (err) {
      setFeedback("", err instanceof Error ? err.message : "Failed to create workflow run");
    } finally {
      setLoadingKey("");
    }
  }

  async function handleLookupRun(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoadingKey("lookup");
    setFeedback("Fetching workflow run...");

    try {
      const response = await fetch(`/api/workflow-runs/${lookupRunId.trim()}`);
      const data = (await readJsonResponse(response)) as WorkflowRunResponse;
      setLookupRunResult(data);
      setFeedback(`Workflow run ${data.id} loaded.`);
    } catch (err) {
      setFeedback("", err instanceof Error ? err.message : "Failed to fetch workflow run");
    } finally {
      setLoadingKey("");
    }
  }

  function updateTask(index: number, field: keyof TaskDefinitionForm, value: string) {
    setTasks((current) =>
      current.map((task, taskIndex) => (taskIndex === index ? { ...task, [field]: value } : task)),
    );
  }

  return (
    <main className="page-shell">
      <section className="hero">
        <p className="eyebrow">Workflow Engine Console</p>
        <h1>Design workflows, launch runs, and inspect execution state.</h1>
        <p className="hero-copy">
          This Next.js frontend talks to the Go backend through local proxy routes, so you can manage workflow
          definitions and runtime instances from one screen.
        </p>
      </section>

      <section className="status-strip">
        <div>
          <span>Status</span>
          <strong>{error ? "Action failed" : message ? "Ready" : "Idle"}</strong>
        </div>
        <p>{error || message || "Create a workflow definition to get started."}</p>
      </section>

      <div className="grid">
        <section className="card card-wide">
          <header className="card-header">
            <div>
              <p className="section-kicker">Step 1</p>
              <h2>Create workflow definition</h2>
            </div>
            <button
              type="button"
              className="ghost-button"
              onClick={() => setTasks((current) => [...current, defaultTask()])}
            >
              Add task row
            </button>
          </header>

          <form className="stack" onSubmit={handleCreateWorkflowDefinition}>
            <div className="field-grid">
              <label>
                Workflow name
                <input value={workflowName} onChange={(event) => setWorkflowName(event.target.value)} />
              </label>
              <label>
                Version
                <input value={workflowVersion} onChange={(event) => setWorkflowVersion(event.target.value)} />
              </label>
            </div>

            <label>
              Description
              <textarea value={workflowDescription} onChange={(event) => setWorkflowDescription(event.target.value)} />
            </label>

            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={workflowActive}
                onChange={(event) => setWorkflowActive(event.target.checked)}
              />
              Create definition as active
            </label>

            <div className="task-stack">
              {tasks.map((task, index) => (
                <article className="task-card" key={`${index}-${task.name}`}>
                  <div className="task-card-header">
                    <strong>Task {index + 1}</strong>
                    {tasks.length > 1 ? (
                      <button
                        type="button"
                        className="ghost-button"
                        onClick={() => setTasks((current) => current.filter((_, taskIndex) => taskIndex !== index))}
                      >
                        Remove
                      </button>
                    ) : null}
                  </div>

                  <div className="field-grid">
                    <label>
                      Name
                      <input value={task.name} onChange={(event) => updateTask(index, "name", event.target.value)} />
                    </label>
                    <label>
                      Kind
                      <select
                        value={task.task_kind}
                        onChange={(event) => updateTask(index, "task_kind", event.target.value)}
                      >
                        {taskKinds.map((kind) => (
                          <option key={kind} value={kind}>
                            {kind}
                          </option>
                        ))}
                      </select>
                    </label>
                  </div>

                  <label>
                    Handler name
                    <input
                      value={task.handler_name}
                      onChange={(event) => updateTask(index, "handler_name", event.target.value)}
                    />
                  </label>

                  <div className="field-grid field-grid-triple">
                    <label>
                      Retry max attempts
                      <input
                        value={task.retry_max_attempts}
                        onChange={(event) => updateTask(index, "retry_max_attempts", event.target.value)}
                      />
                    </label>
                    <label>
                      Retry backoff seconds
                      <input
                        value={task.retry_backoff_seconds}
                        onChange={(event) => updateTask(index, "retry_backoff_seconds", event.target.value)}
                      />
                    </label>
                    <label>
                      Timeout seconds
                      <input
                        value={task.timeout_seconds}
                        onChange={(event) => updateTask(index, "timeout_seconds", event.target.value)}
                      />
                    </label>
                  </div>

                  <label>
                    Config JSON
                    <textarea value={task.config} onChange={(event) => updateTask(index, "config", event.target.value)} />
                  </label>
                </article>
              ))}
            </div>

            <button className="primary-button" disabled={loadingKey === "definition"} type="submit">
              {loadingKey === "definition" ? "Creating..." : "Create workflow definition"}
            </button>
          </form>
        </section>

        <section className="card">
          <header className="card-header">
            <div>
              <p className="section-kicker">Step 2</p>
              <h2>Append task definition</h2>
            </div>
          </header>

          <form className="stack" onSubmit={handleCreateTaskDefinition}>
            <label>
              Workflow definition ID
              <input value={selectedWorkflowId} onChange={(event) => setSelectedWorkflowId(event.target.value)} />
            </label>
            <label>
              Task name
              <input value={newTask.name} onChange={(event) => setNewTask({ ...newTask, name: event.target.value })} />
            </label>
            <div className="field-grid">
              <label>
                Kind
                <select
                  value={newTask.task_kind}
                  onChange={(event) => setNewTask({ ...newTask, task_kind: event.target.value })}
                >
                  {taskKinds.map((kind) => (
                    <option key={kind} value={kind}>
                      {kind}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Handler name
                <input
                  value={newTask.handler_name}
                  onChange={(event) => setNewTask({ ...newTask, handler_name: event.target.value })}
                />
              </label>
            </div>
            <label>
              Config JSON
              <textarea value={newTask.config} onChange={(event) => setNewTask({ ...newTask, config: event.target.value })} />
            </label>

            <button className="primary-button" disabled={loadingKey === "task"} type="submit">
              {loadingKey === "task" ? "Adding..." : "Append task"}
            </button>
          </form>
        </section>

        <section className="card">
          <header className="card-header">
            <div>
              <p className="section-kicker">Step 3</p>
              <h2>Start workflow run</h2>
            </div>
          </header>

          <form className="stack" onSubmit={handleCreateWorkflowRun}>
            <label>
              Workflow definition ID
              <input
                value={runWorkflowDefinitionId}
                onChange={(event) => setRunWorkflowDefinitionId(event.target.value)}
              />
            </label>
            <label>
              Business ID
              <input value={businessId} onChange={(event) => setBusinessId(event.target.value)} />
            </label>
            <label>
              Input payload JSON
              <textarea value={inputPayload} onChange={(event) => setInputPayload(event.target.value)} />
            </label>

            <button className="primary-button" disabled={loadingKey === "run"} type="submit">
              {loadingKey === "run" ? "Starting..." : "Create workflow run"}
            </button>
          </form>
        </section>

        <section className="card">
          <header className="card-header">
            <div>
              <p className="section-kicker">Step 4</p>
              <h2>Inspect workflow run</h2>
            </div>
          </header>

          <form className="stack" onSubmit={handleLookupRun}>
            <label>
              Workflow run ID
              <input value={lookupRunId} onChange={(event) => setLookupRunId(event.target.value)} />
            </label>
            <button className="primary-button" disabled={loadingKey === "lookup"} type="submit">
              {loadingKey === "lookup" ? "Loading..." : "Fetch workflow run"}
            </button>
          </form>
        </section>
      </div>

      <section className="results-grid">
        <ResultPanel title="Workflow definition result" data={definitionResult} />
        <ResultPanel title="Task append result" data={taskResult} />
        <ResultPanel title="Workflow run create result" data={runResult} />
        <ResultPanel title="Workflow run lookup result" data={lookupRunResult} />
      </section>
    </main>
  );
}

function ResultPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <article className="result-card">
      <h3>{title}</h3>
      <pre>{data ? JSON.stringify(data, null, 2) : "No result yet."}</pre>
    </article>
  );
}
