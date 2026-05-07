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
