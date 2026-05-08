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

func getNextTaskStepOrder(ctx context.Context, tx *sql.Tx, workflowID string) (int, error) {
	var lockedWorkflowID string
	err := tx.QueryRowContext(
		ctx,
		`select id from workflow_definitions where id = $1 for update`,
		workflowID,
	).Scan(&lockedWorkflowID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrWorkflowNotFound
		}
		return 0, fmt.Errorf("lock workflow definition: %w", err)
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

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return value
}
