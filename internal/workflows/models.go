package workflows

import "time"

type WorkflowDefinition struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     int              `json:"version"`
	Description string           `json:"description,omitempty"`
	IsActive    bool             `json:"is_active"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Tasks       []TaskDefinition `json:"tasks"`
}

type TaskDefinition struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	StepOrder           int            `json:"step_order"`
	TaskKind            string         `json:"task_kind"`
	HandlerName         string         `json:"handler_name"`
	RetryMaxAttempts    int            `json:"retry_max_attempts"`
	RetryBackoffSeconds int            `json:"retry_backoff_seconds"`
	TimeoutSeconds      *int           `json:"timeout_seconds,omitempty"`
	Config              map[string]any `json:"config,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type WorkflowRun struct {
	ID                   string         `json:"id"`
	WorkflowDefinitionID string         `json:"workflow_definition_id"`
	BusinessID           string         `json:"business_id,omitempty"`
	Status               string         `json:"status"`
	CurrentStepOrder     *int           `json:"current_step_order,omitempty"`
	InputPayload         map[string]any `json:"input_payload"`
	OutputPayload        map[string]any `json:"output_payload,omitempty"`
	ErrorMessage         string         `json:"error_message,omitempty"`
	StartedAt            *time.Time     `json:"started_at,omitempty"`
	CompletedAt          *time.Time     `json:"completed_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	TaskRuns             []TaskRun      `json:"task_runs"`
}

type TaskRun struct {
	ID               string         `json:"id"`
	WorkflowRunID    string         `json:"workflow_run_id"`
	TaskDefinitionID string         `json:"task_definition_id"`
	StepOrder        int            `json:"step_order"`
	Status           string         `json:"status"`
	AttemptCount     int            `json:"attempt_count"`
	InputPayload     map[string]any `json:"input_payload"`
	OutputPayload    map[string]any `json:"output_payload,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	ScheduledAt      *time.Time     `json:"scheduled_at,omitempty"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	LeaseOwner       string         `json:"lease_owner,omitempty"`
	LeaseExpiresAt   *time.Time     `json:"lease_expires_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type CreateWorkflowDefinitionRequest struct {
	Name        string              `json:"name"`
	Version     int                 `json:"version"`
	Description string              `json:"description"`
	IsActive    *bool               `json:"is_active"`
	Tasks       []CreateTaskRequest `json:"tasks"`
}

type CreateTaskRequest struct {
	Name                string         `json:"name"`
	TaskKind            string         `json:"task_kind"`
	HandlerName         string         `json:"handler_name"`
	RetryMaxAttempts    *int           `json:"retry_max_attempts"`
	RetryBackoffSeconds *int           `json:"retry_backoff_seconds"`
	TimeoutSeconds      *int           `json:"timeout_seconds"`
	Config              map[string]any `json:"config"`
}

type CreateWorkflowRunRequest struct {
	WorkflowDefinitionID string         `json:"workflow_definition_id"`
	BusinessID           string         `json:"business_id"`
	InputPayload         map[string]any `json:"input_payload"`
}
