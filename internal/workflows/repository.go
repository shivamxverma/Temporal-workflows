package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

type SQLRepository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) CreateWorkflowDefinition(ctx context.Context, params CreateWorkflowDefinitionParams) (WorkflowDefinition, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return WorkflowDefinition{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	workflow, err := insertWorkflowDefinition(ctx, tx, params)
	if err != nil {
		return WorkflowDefinition{}, err
	}

	tasks, err := insertTaskDefinitions(ctx, tx, workflow.ID, params.Tasks)
	if err != nil {
		return WorkflowDefinition{}, err
	}

	workflow.Tasks = tasks

	if err := tx.Commit(); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("commit tx: %w", err)
	}

	return workflow, nil
}

func (r *SQLRepository) CreateTaskDefinition(ctx context.Context, workflowID string, params CreateTaskParams) (TaskDefinition, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return TaskDefinition{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	nextStepOrder, err := getNextTaskStepOrder(ctx, tx, workflowID)
	if err != nil {
		return TaskDefinition{}, err
	}

	params.StepOrder = nextStepOrder
	tasks, err := insertTaskDefinitions(ctx, tx, workflowID, []CreateTaskParams{params})
	if err != nil {
		return TaskDefinition{}, err
	}

	if err := tx.Commit(); err != nil {
		return TaskDefinition{}, fmt.Errorf("commit tx: %w", err)
	}

	return tasks[0], nil
}

func (r *SQLRepository) CreateWorkflowRun(ctx context.Context, params CreateWorkflowRunParams) (WorkflowRun, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	workflowDefinition, err := getWorkflowDefinitionForUpdate(ctx, tx, params.WorkflowDefinitionID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if !workflowDefinition.IsActive {
		return WorkflowRun{}, ErrWorkflowDefinitionInactive
	}

	taskDefinitions, err := loadTaskDefinitionsForWorkflow(ctx, tx, workflowDefinition.ID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if len(taskDefinitions) == 0 {
		return WorkflowRun{}, ErrWorkflowDefinitionHasNoTasks
	}

	workflowRun, err := insertWorkflowRun(ctx, tx, params)
	if err != nil {
		return WorkflowRun{}, err
	}

	taskRuns, err := insertTaskRuns(ctx, tx, workflowRun.ID, taskDefinitions)
	if err != nil {
		return WorkflowRun{}, err
	}

	workflowRun.TaskRuns = taskRuns

	if err := insertInitialWorkflowEvents(ctx, tx, workflowRun, taskRuns[0]); err != nil {
		return WorkflowRun{}, err
	}

	if err := tx.Commit(); err != nil {
		return WorkflowRun{}, fmt.Errorf("commit tx: %w", err)
	}

	return workflowRun, nil
}

func (r *SQLRepository) ListWorkflowDefinitions(ctx context.Context, includeInactive bool) ([]WorkflowDefinition, error) {
	query := `
		select
			wd.id,
			wd.name,
			wd.version,
			coalesce(wd.description, ''),
			wd.is_active,
			wd.created_at,
			wd.updated_at
		from workflow_definitions wd
	`
	var args []any
	if !includeInactive {
		query += " where wd.is_active = true"
	}
	query += " order by wd.name asc, wd.version desc"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workflows: %w", err)
	}
	defer rows.Close()

	workflowsByID := make(map[string]*WorkflowDefinition)
	orderedIDs := make([]string, 0)
	for rows.Next() {
		var workflow WorkflowDefinition
		if err := rows.Scan(
			&workflow.ID,
			&workflow.Name,
			&workflow.Version,
			&workflow.Description,
			&workflow.IsActive,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}

		workflow.Tasks = []TaskDefinition{}
		workflowsByID[workflow.ID] = &workflow
		orderedIDs = append(orderedIDs, workflow.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflows: %w", err)
	}

	if len(orderedIDs) == 0 {
		return []WorkflowDefinition{}, nil
	}

	if err := r.loadTasks(ctx, workflowsByID); err != nil {
		return nil, err
	}

	result := make([]WorkflowDefinition, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, *workflowsByID[id])
	}

	return result, nil
}

func (r *SQLRepository) GetWorkflowDefinition(ctx context.Context, id string) (WorkflowDefinition, error) {
	query := `
		select
			id,
			name,
			version,
			coalesce(description, ''),
			is_active,
			created_at,
			updated_at
		from workflow_definitions
		where id = $1
	`

	var workflow WorkflowDefinition
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&workflow.ID,
		&workflow.Name,
		&workflow.Version,
		&workflow.Description,
		&workflow.IsActive,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowDefinition{}, ErrWorkflowNotFound
		}
		return WorkflowDefinition{}, fmt.Errorf("get workflow: %w", err)
	}

	workflowsByID := map[string]*WorkflowDefinition{
		workflow.ID: &workflow,
	}
	if err := r.loadTasks(ctx, workflowsByID); err != nil {
		return WorkflowDefinition{}, err
	}

	return workflow, nil
}

func (r *SQLRepository) GetWorkflowRun(ctx context.Context, id string) (WorkflowRun, error) {
	query := `
		select
			id,
			workflow_definition_id,
			coalesce(business_id, ''),
			status,
			current_step_order,
			input_payload,
			output_payload,
			coalesce(error_message, ''),
			started_at,
			completed_at,
			created_at,
			updated_at
		from workflow_runs
		where id = $1
	`

	var workflowRun WorkflowRun
	var inputPayloadBytes []byte
	var outputPayloadBytes []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&workflowRun.ID,
		&workflowRun.WorkflowDefinitionID,
		&workflowRun.BusinessID,
		&workflowRun.Status,
		&workflowRun.CurrentStepOrder,
		&inputPayloadBytes,
		&outputPayloadBytes,
		&workflowRun.ErrorMessage,
		&workflowRun.StartedAt,
		&workflowRun.CompletedAt,
		&workflowRun.CreatedAt,
		&workflowRun.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowRun{}, ErrWorkflowRunNotFound
		}
		return WorkflowRun{}, fmt.Errorf("get workflow run: %w", err)
	}

	workflowRun.InputPayload, err = decodeConfig(inputPayloadBytes)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("decode workflow run input payload: %w", err)
	}
	workflowRun.OutputPayload, err = decodeNullableConfig(outputPayloadBytes)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("decode workflow run output payload: %w", err)
	}

	workflowRun.TaskRuns, err = r.loadTaskRuns(ctx, workflowRun.ID)
	if err != nil {
		return WorkflowRun{}, err
	}

	return workflowRun, nil
}

func insertWorkflowDefinition(ctx context.Context, tx *sql.Tx, params CreateWorkflowDefinitionParams) (WorkflowDefinition, error) {
	query := `
		insert into workflow_definitions (
			name,
			version,
			description,
			is_active
		) values ($1, $2, $3, $4)
		returning
			id,
			name,
			version,
			coalesce(description, ''),
			is_active,
			created_at,
			updated_at
	`

	var workflow WorkflowDefinition
	err := tx.QueryRowContext(
		ctx,
		query,
		params.Name,
		params.Version,
		nullIfEmpty(params.Description),
		params.IsActive,
	).Scan(
		&workflow.ID,
		&workflow.Name,
		&workflow.Version,
		&workflow.Description,
		&workflow.IsActive,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return WorkflowDefinition{}, ErrWorkflowAlreadyExists
		}
		return WorkflowDefinition{}, fmt.Errorf("insert workflow definition: %w", err)
	}

	return workflow, nil
}

func insertTaskDefinitions(ctx context.Context, tx *sql.Tx, workflowID string, tasks []CreateTaskParams) ([]TaskDefinition, error) {
	query := `
		insert into task_definitions (
			workflow_definition_id,
			name,
			step_order,
			task_kind,
			handler_name,
			retry_max_attempts,
			retry_backoff_seconds,
			timeout_seconds,
			config
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning
			id,
			name,
			step_order,
			task_kind,
			handler_name,
			retry_max_attempts,
			retry_backoff_seconds,
			timeout_seconds,
			config,
			created_at,
			updated_at
	`

	definitions := make([]TaskDefinition, 0, len(tasks))
	for _, task := range tasks {
		rawConfig, err := json.Marshal(task.Config)
		if err != nil {
			return nil, fmt.Errorf("marshal task config for %q: %w", task.Name, err)
		}

		var definition TaskDefinition
		var configBytes []byte
		err = tx.QueryRowContext(
			ctx,
			query,
			workflowID,
			task.Name,
			task.StepOrder,
			task.TaskKind,
			task.HandlerName,
			task.RetryMaxAttempts,
			task.RetryBackoffSeconds,
			task.TimeoutSeconds,
			rawConfig,
		).Scan(
			&definition.ID,
			&definition.Name,
			&definition.StepOrder,
			&definition.TaskKind,
			&definition.HandlerName,
			&definition.RetryMaxAttempts,
			&definition.RetryBackoffSeconds,
			&definition.TimeoutSeconds,
			&configBytes,
			&definition.CreatedAt,
			&definition.UpdatedAt,
		)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrTaskAlreadyExists
			}
			return nil, fmt.Errorf("insert task definition %q: %w", task.Name, err)
		}

		definition.Config, err = decodeConfig(configBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task config %q: %w", task.Name, err)
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func getWorkflowDefinitionForUpdate(ctx context.Context, tx *sql.Tx, workflowID string) (WorkflowDefinition, error) {
	query := `
		select
			id,
			name,
			version,
			coalesce(description, ''),
			is_active,
			created_at,
			updated_at
		from workflow_definitions
		where id = $1
		for update
	`

	var workflowDefinition WorkflowDefinition
	err := tx.QueryRowContext(ctx, query, workflowID).Scan(
		&workflowDefinition.ID,
		&workflowDefinition.Name,
		&workflowDefinition.Version,
		&workflowDefinition.Description,
		&workflowDefinition.IsActive,
		&workflowDefinition.CreatedAt,
		&workflowDefinition.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowDefinition{}, ErrWorkflowNotFound
		}
		return WorkflowDefinition{}, fmt.Errorf("lock workflow definition: %w", err)
	}

	return workflowDefinition, nil
}

func loadTaskDefinitionsForWorkflow(ctx context.Context, tx *sql.Tx, workflowID string) ([]TaskDefinition, error) {
	query := `
		select
			id,
			name,
			step_order,
			task_kind,
			handler_name,
			retry_max_attempts,
			retry_backoff_seconds,
			timeout_seconds,
			config,
			created_at,
			updated_at
		from task_definitions
		where workflow_definition_id = $1
		order by step_order asc
	`

	rows, err := tx.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("query task definitions for workflow: %w", err)
	}
	defer rows.Close()

	taskDefinitions := make([]TaskDefinition, 0)
	for rows.Next() {
		var taskDefinition TaskDefinition
		var configBytes []byte
		if err := rows.Scan(
			&taskDefinition.ID,
			&taskDefinition.Name,
			&taskDefinition.StepOrder,
			&taskDefinition.TaskKind,
			&taskDefinition.HandlerName,
			&taskDefinition.RetryMaxAttempts,
			&taskDefinition.RetryBackoffSeconds,
			&taskDefinition.TimeoutSeconds,
			&configBytes,
			&taskDefinition.CreatedAt,
			&taskDefinition.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task definition for workflow: %w", err)
		}

		taskDefinition.Config, err = decodeConfig(configBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task definition config: %w", err)
		}

		taskDefinitions = append(taskDefinitions, taskDefinition)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task definitions for workflow: %w", err)
	}

	return taskDefinitions, nil
}

func insertWorkflowRun(ctx context.Context, tx *sql.Tx, params CreateWorkflowRunParams) (WorkflowRun, error) {
	query := `
		insert into workflow_runs (
			workflow_definition_id,
			business_id,
			status,
			current_step_order,
			input_payload,
			started_at
		) values ($1, $2, $3, $4, $5, now())
		returning
			id,
			workflow_definition_id,
			coalesce(business_id, ''),
			status,
			current_step_order,
			input_payload,
			output_payload,
			coalesce(error_message, ''),
			started_at,
			completed_at,
			created_at,
			updated_at
	`

	rawInputPayload, err := json.Marshal(params.InputPayload)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("marshal workflow run input payload: %w", err)
	}

	var workflowRun WorkflowRun
	var inputPayloadBytes []byte
	var outputPayloadBytes []byte
	err = tx.QueryRowContext(
		ctx,
		query,
		params.WorkflowDefinitionID,
		nullIfEmpty(params.BusinessID),
		"running",
		1,
		rawInputPayload,
	).Scan(
		&workflowRun.ID,
		&workflowRun.WorkflowDefinitionID,
		&workflowRun.BusinessID,
		&workflowRun.Status,
		&workflowRun.CurrentStepOrder,
		&inputPayloadBytes,
		&outputPayloadBytes,
		&workflowRun.ErrorMessage,
		&workflowRun.StartedAt,
		&workflowRun.CompletedAt,
		&workflowRun.CreatedAt,
		&workflowRun.UpdatedAt,
	)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("insert workflow run: %w", err)
	}

	workflowRun.InputPayload, err = decodeConfig(inputPayloadBytes)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("decode workflow run input payload: %w", err)
	}
	workflowRun.OutputPayload, err = decodeNullableConfig(outputPayloadBytes)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("decode workflow run output payload: %w", err)
	}
	workflowRun.TaskRuns = []TaskRun{}

	return workflowRun, nil
}

func insertTaskRuns(ctx context.Context, tx *sql.Tx, workflowRunID string, taskDefinitions []TaskDefinition) ([]TaskRun, error) {
	query := `
		insert into task_runs (
			workflow_run_id,
			task_definition_id,
			step_order,
			status,
			attempt_count,
			input_payload,
			scheduled_at
		) values ($1, $2, $3, $4, $5, $6, $7)
		returning
			id,
			workflow_run_id,
			task_definition_id,
			step_order,
			status,
			attempt_count,
			input_payload,
			output_payload,
			coalesce(error_message, ''),
			scheduled_at,
			started_at,
			completed_at,
			coalesce(lease_owner, ''),
			lease_expires_at,
			created_at,
			updated_at
	`

	taskRuns := make([]TaskRun, 0, len(taskDefinitions))
	emptyPayload := []byte(`{}`)

	for i, taskDefinition := range taskDefinitions {
		status := "pending"
		var scheduledAt any
		if i == 0 {
			status = "scheduled"
			scheduledAt = time.Now().UTC()
		}

		var taskRun TaskRun
		var inputPayloadBytes []byte
		var outputPayloadBytes []byte
		err := tx.QueryRowContext(
			ctx,
			query,
			workflowRunID,
			taskDefinition.ID,
			taskDefinition.StepOrder,
			status,
			0,
			emptyPayload,
			scheduledAt,
		).Scan(
			&taskRun.ID,
			&taskRun.WorkflowRunID,
			&taskRun.TaskDefinitionID,
			&taskRun.StepOrder,
			&taskRun.Status,
			&taskRun.AttemptCount,
			&inputPayloadBytes,
			&outputPayloadBytes,
			&taskRun.ErrorMessage,
			&taskRun.ScheduledAt,
			&taskRun.StartedAt,
			&taskRun.CompletedAt,
			&taskRun.LeaseOwner,
			&taskRun.LeaseExpiresAt,
			&taskRun.CreatedAt,
			&taskRun.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert task run for step %d: %w", taskDefinition.StepOrder, err)
		}

		taskRun.InputPayload, err = decodeConfig(inputPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task run input payload for step %d: %w", taskDefinition.StepOrder, err)
		}
		taskRun.OutputPayload, err = decodeNullableConfig(outputPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task run output payload for step %d: %w", taskDefinition.StepOrder, err)
		}

		taskRuns = append(taskRuns, taskRun)
	}

	return taskRuns, nil
}

func insertInitialWorkflowEvents(ctx context.Context, tx *sql.Tx, workflowRun WorkflowRun, firstTaskRun TaskRun) error {
	workflowStartedPayload := map[string]any{
		"workflow_definition_id": workflowRun.WorkflowDefinitionID,
		"status":                 workflowRun.Status,
		"current_step_order":     workflowRun.CurrentStepOrder,
	}
	if err := insertWorkflowEvent(ctx, tx, workflowRun.ID, nil, 1, "workflow_started", workflowStartedPayload); err != nil {
		return err
	}

	taskScheduledPayload := map[string]any{
		"task_definition_id": firstTaskRun.TaskDefinitionID,
		"step_order":         firstTaskRun.StepOrder,
		"status":             firstTaskRun.Status,
	}
	if err := insertWorkflowEvent(ctx, tx, workflowRun.ID, &firstTaskRun.ID, 2, "task_scheduled", taskScheduledPayload); err != nil {
		return err
	}

	return nil
}

func insertWorkflowEvent(ctx context.Context, tx *sql.Tx, workflowRunID string, taskRunID *string, sequenceNumber int64, eventType string, payload map[string]any) error {
	query := `
		insert into workflow_events (
			workflow_run_id,
			task_run_id,
			sequence_number,
			event_type,
			payload
		) values ($1, $2, $3, $4, $5)
	`

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal workflow event payload: %w", err)
	}

	if _, err := tx.ExecContext(ctx, query, workflowRunID, taskRunID, sequenceNumber, eventType, rawPayload); err != nil {
		return fmt.Errorf("insert workflow event %q: %w", eventType, err)
	}

	return nil
}

func getNextTaskStepOrder(ctx context.Context, tx *sql.Tx, workflowID string) (int, error) {
	if _, err := getWorkflowDefinitionForUpdate(ctx, tx, workflowID); err != nil {
		return 0, err
	}

	var currentMax int
	err = tx.QueryRowContext(
		ctx,
		`select coalesce(max(step_order), 0) from task_definitions where workflow_definition_id = $1`,
		workflowID,
	).Scan(&currentMax)
	if err != nil {
		return 0, fmt.Errorf("get max task step order: %w", err)
	}

	return currentMax + 1, nil
}

func (r *SQLRepository) loadTasks(ctx context.Context, workflowsByID map[string]*WorkflowDefinition) error {
	ids := make([]string, 0, len(workflowsByID))
	for id := range workflowsByID {
		ids = append(ids, id)
	}

	query := `
		select
			workflow_definition_id,
			id,
			name,
			step_order,
			task_kind,
			handler_name,
			retry_max_attempts,
			retry_backoff_seconds,
			timeout_seconds,
			config,
			created_at,
			updated_at
		from task_definitions
		where workflow_definition_id = any($1)
		order by workflow_definition_id asc, step_order asc
	`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("query task definitions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var workflowID string
		var task TaskDefinition
		var configBytes []byte

		if err := rows.Scan(
			&workflowID,
			&task.ID,
			&task.Name,
			&task.StepOrder,
			&task.TaskKind,
			&task.HandlerName,
			&task.RetryMaxAttempts,
			&task.RetryBackoffSeconds,
			&task.TimeoutSeconds,
			&configBytes,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return fmt.Errorf("scan task definition: %w", err)
		}

		task.Config, err = decodeConfig(configBytes)
		if err != nil {
			return fmt.Errorf("decode task definition config: %w", err)
		}

		workflow := workflowsByID[workflowID]
		workflow.Tasks = append(workflow.Tasks, task)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task definitions: %w", err)
	}

	return nil
}

func decodeConfig(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config == nil {
		return map[string]any{}, nil
	}

	return config, nil
}

func decodeNullableConfig(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func (r *SQLRepository) loadTaskRuns(ctx context.Context, workflowRunID string) ([]TaskRun, error) {
	query := `
		select
			id,
			workflow_run_id,
			task_definition_id,
			step_order,
			status,
			attempt_count,
			input_payload,
			output_payload,
			coalesce(error_message, ''),
			scheduled_at,
			started_at,
			completed_at,
			coalesce(lease_owner, ''),
			lease_expires_at,
			created_at,
			updated_at
		from task_runs
		where workflow_run_id = $1
		order by step_order asc
	`

	rows, err := r.db.QueryContext(ctx, query, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("query task runs: %w", err)
	}
	defer rows.Close()

	taskRuns := make([]TaskRun, 0)
	for rows.Next() {
		var taskRun TaskRun
		var inputPayloadBytes []byte
		var outputPayloadBytes []byte
		if err := rows.Scan(
			&taskRun.ID,
			&taskRun.WorkflowRunID,
			&taskRun.TaskDefinitionID,
			&taskRun.StepOrder,
			&taskRun.Status,
			&taskRun.AttemptCount,
			&inputPayloadBytes,
			&outputPayloadBytes,
			&taskRun.ErrorMessage,
			&taskRun.ScheduledAt,
			&taskRun.StartedAt,
			&taskRun.CompletedAt,
			&taskRun.LeaseOwner,
			&taskRun.LeaseExpiresAt,
			&taskRun.CreatedAt,
			&taskRun.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task run: %w", err)
		}

		taskRun.InputPayload, err = decodeConfig(inputPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task run input payload: %w", err)
		}
		taskRun.OutputPayload, err = decodeNullableConfig(outputPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("decode task run output payload: %w", err)
		}

		taskRuns = append(taskRuns, taskRun)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task runs: %w", err)
	}

	return taskRuns, nil
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return value
}
