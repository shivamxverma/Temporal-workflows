package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"workflow-engine-mvp/internal/workflows"
)

var ErrServerClosed = http.ErrServerClosed

type Handler struct {
	service *workflows.Service
	logger  *log.Logger
}

func NewHandler(service *workflows.Service, logger *log.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

func (h *Handler) Server(addr string) *http.Server {
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", h.handleHealth)

	v1 := router.Group("/api/v1")
	v1.POST("/workflow-definitions", h.handleCreateWorkflowDefinition)
	v1.GET("/workflow-definitions/:id", h.handleGetWorkflowDefinition)
	v1.POST("/workflow-definitions/:id/task-definitions", h.handleCreateTaskDefinition)

	return &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func (h *Handler) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) handleCreateWorkflowDefinition(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var req workflows.CreateWorkflowDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	workflow, err := h.service.CreateWorkflowDefinition(ctx, req)
	if err != nil {
		h.writeWorkflowError(c, err)
		return
	}

	c.JSON(http.StatusCreated, workflow)
}

func (h *Handler) handleGetWorkflowDefinition(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := c.Param("id")
	workflow, err := h.service.GetWorkflowDefinition(ctx, id)
	if err != nil {
		h.writeWorkflowError(c, err)
		return
	}

	c.JSON(http.StatusOK, workflow)
}

func (h *Handler) handleCreateTaskDefinition(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var req workflows.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	task, err := h.service.CreateTaskDefinition(ctx, c.Param("id"), req)
	if err != nil {
		h.writeWorkflowError(c, err)
		return
	}

	c.JSON(http.StatusCreated, task)
}

func (h *Handler) writeWorkflowError(c *gin.Context, err error) {
	var validationErr workflows.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeError(c, http.StatusBadRequest, "validation_error", validationErr.Error())
	case errors.Is(err, workflows.ErrWorkflowAlreadyExists):
		writeError(c, http.StatusConflict, "workflow_already_exists", err.Error())
	case errors.Is(err, workflows.ErrTaskAlreadyExists):
		writeError(c, http.StatusConflict, "task_already_exists", err.Error())
	case errors.Is(err, workflows.ErrWorkflowNotFound):
		writeError(c, http.StatusNotFound, "workflow_not_found", err.Error())
	default:
		h.logger.Printf("unexpected error: %v", err)
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, errorResponse{
		Code:    code,
		Message: message,
	})
}
