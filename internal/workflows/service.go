package workflows

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	ErrWorkflowAlreadyExists       = errors.New("workflow definition with same name and version already exists")
	ErrWorkflowNotFound            = errors.New("workflow definition not found")
	ErrTaskAlreadyExists           = errors.New("task definition with same name already exists in workflow definition")
	ErrWorkflowDefinitionInactive  = errors.New("workflow definition is inactive")
	ErrWorkflowDefinitionHasNoTasks = errors.New("workflow definition has no task definitions")
	ErrWorkflowRunNotFound         = errors.New("workflow run not found")
)

var allowedTaskKinds = []string{"system", "executor", "persistence", "notification"}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type Repository interface {
	CreateWorkflowDefinition(ctx context.Context, params CreateWorkflowDefinitionParams) (WorkflowDefinition, error)
	CreateTaskDefinition(ctx context.Context, workflowID string, params CreateTaskParams) (TaskDefinition, error)
	CreateWorkflowRun(ctx context.Context, params CreateWorkflowRunParams) (WorkflowRun, error)
	ListWorkflowDefinitions(ctx context.Context, includeInactive bool) ([]WorkflowDefinition, error)
	GetWorkflowDefinition(ctx context.Context, id string) (WorkflowDefinition, error)
	GetWorkflowRun(ctx context.Context, id string) (WorkflowRun, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateWorkflowDefinition(ctx context.Context, req CreateWorkflowDefinitionRequest) (WorkflowDefinition, error) {
	params, err := validateCreateWorkflowDefinition(req)
	if err != nil {
		return WorkflowDefinition{}, err
	}

	return s.repo.CreateWorkflowDefinition(ctx, params)
}

func (s *Service) CreateTaskDefinition(ctx context.Context, workflowID string, req CreateTaskRequest) (TaskDefinition, error) {
	if strings.TrimSpace(workflowID) == "" {
		return TaskDefinition{}, ValidationError{Message: "workflow id is required"}
	}

	params, err := validateCreateTaskRequest(req, "task")
	if err != nil {
		return TaskDefinition{}, err
	}

	return s.repo.CreateTaskDefinition(ctx, workflowID, params)
}

func (s *Service) CreateWorkflowRun(ctx context.Context, req CreateWorkflowRunRequest) (WorkflowRun, error) {
	params, err := validateCreateWorkflowRun(req)
	if err != nil {
		return WorkflowRun{}, err
	}

	return s.repo.CreateWorkflowRun(ctx, params)
}

func (s *Service) ListWorkflowDefinitions(ctx context.Context, includeInactive bool) ([]WorkflowDefinition, error) {
	return s.repo.ListWorkflowDefinitions(ctx, includeInactive)
}

func (s *Service) GetWorkflowDefinition(ctx context.Context, id string) (WorkflowDefinition, error) {
	if strings.TrimSpace(id) == "" {
		return WorkflowDefinition{}, ValidationError{Message: "workflow id is required"}
	}

	return s.repo.GetWorkflowDefinition(ctx, id)
}

func (s *Service) GetWorkflowRun(ctx context.Context, id string) (WorkflowRun, error) {
	if strings.TrimSpace(id) == "" {
		return WorkflowRun{}, ValidationError{Message: "workflow run id is required"}
	}

	return s.repo.GetWorkflowRun(ctx, id)
}

type CreateWorkflowDefinitionParams struct {
	Name        string
	Version     int
	Description string
	IsActive    bool
	Tasks       []CreateTaskParams
}

type CreateTaskParams struct {
	Name                string
	StepOrder           int
	TaskKind            string
	HandlerName         string
	RetryMaxAttempts    int
	RetryBackoffSeconds int
	TimeoutSeconds      *int
	Config              map[string]any
}

type CreateWorkflowRunParams struct {
	WorkflowDefinitionID string
	BusinessID           string
	InputPayload         map[string]any
}

func validateCreateWorkflowDefinition(req CreateWorkflowDefinitionRequest) (CreateWorkflowDefinitionParams, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CreateWorkflowDefinitionParams{}, ValidationError{Message: "workflow name is required"}
	}

	if len(req.Tasks) == 0 {
		return CreateWorkflowDefinitionParams{}, ValidationError{Message: "at least one task is required"}
	}

	version := req.Version
	if version == 0 {
		version = 1
	}
	if version < 1 {
		return CreateWorkflowDefinitionParams{}, ValidationError{Message: "workflow version must be at least 1"}
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	taskNames := make(map[string]struct{}, len(req.Tasks))
	tasks := make([]CreateTaskParams, 0, len(req.Tasks))
	for i, task := range req.Tasks {
		params, err := validateCreateTaskRequest(task, fmt.Sprintf("task %d", i+1))
		if err != nil {
			return CreateWorkflowDefinitionParams{}, err
		}

		if _, exists := taskNames[params.Name]; exists {
			return CreateWorkflowDefinitionParams{}, ValidationError{Message: fmt.Sprintf("task %q is duplicated", params.Name)}
		}
		taskNames[params.Name] = struct{}{}
		params.StepOrder = i + 1
		tasks = append(tasks, params)
	}

	return CreateWorkflowDefinitionParams{
		Name:        name,
		Version:     version,
		Description: strings.TrimSpace(req.Description),
		IsActive:    isActive,
		Tasks:       tasks,
	}, nil
}

func validateCreateTaskRequest(task CreateTaskRequest, taskLabel string) (CreateTaskParams, error) {
	name := strings.TrimSpace(task.Name)
	if name == "" {
		return CreateTaskParams{}, ValidationError{Message: fmt.Sprintf("%s: name is required", taskLabel)}
	}

	taskKind := strings.TrimSpace(task.TaskKind)
	if !slices.Contains(allowedTaskKinds, taskKind) {
		return CreateTaskParams{}, ValidationError{
			Message: fmt.Sprintf("task %q: task_kind must be one of %s", name, strings.Join(allowedTaskKinds, ", ")),
		}
	}

	handlerName := strings.TrimSpace(task.HandlerName)
	if handlerName == "" {
		return CreateTaskParams{}, ValidationError{Message: fmt.Sprintf("task %q: handler_name is required", name)}
	}

	retryMaxAttempts := 3
	if task.RetryMaxAttempts != nil {
		retryMaxAttempts = *task.RetryMaxAttempts
	}
	if retryMaxAttempts < 0 {
		return CreateTaskParams{}, ValidationError{Message: fmt.Sprintf("task %q: retry_max_attempts cannot be negative", name)}
	}

	retryBackoffSeconds := 30
	if task.RetryBackoffSeconds != nil {
		retryBackoffSeconds = *task.RetryBackoffSeconds
	}
	if retryBackoffSeconds < 0 {
		return CreateTaskParams{}, ValidationError{Message: fmt.Sprintf("task %q: retry_backoff_seconds cannot be negative", name)}
	}

	if task.TimeoutSeconds != nil && *task.TimeoutSeconds <= 0 {
		return CreateTaskParams{}, ValidationError{Message: fmt.Sprintf("task %q: timeout_seconds must be greater than 0", name)}
	}

	config := task.Config
	if config == nil {
		config = map[string]any{}
	}

	return CreateTaskParams{
		Name:                name,
		TaskKind:            taskKind,
		HandlerName:         handlerName,
		RetryMaxAttempts:    retryMaxAttempts,
		RetryBackoffSeconds: retryBackoffSeconds,
		TimeoutSeconds:      task.TimeoutSeconds,
		Config:              config,
	}, nil
}

func validateCreateWorkflowRun(req CreateWorkflowRunRequest) (CreateWorkflowRunParams, error) {
	workflowDefinitionID := strings.TrimSpace(req.WorkflowDefinitionID)
	if workflowDefinitionID == "" {
		return CreateWorkflowRunParams{}, ValidationError{Message: "workflow_definition_id is required"}
	}

	inputPayload := req.InputPayload
	if inputPayload == nil {
		inputPayload = map[string]any{}
	}

	return CreateWorkflowRunParams{
		WorkflowDefinitionID: workflowDefinitionID,
		BusinessID:           strings.TrimSpace(req.BusinessID),
		InputPayload:         inputPayload,
	}, nil
}
