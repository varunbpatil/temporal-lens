//nolint:testpackage // These tests exercise the service's internal persistence and batching invariants.
package service

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/mocks"
)

func TestTerminalProgressPersistsOnlyMatchingGeneration(t *testing.T) {
	t.Parallel()
	progressPath := filepath.Join(t.TempDir(), "progress.gob")
	progress, err := newTerminalProgress(progressPath, "workflows-1.1-")
	require.NoError(t, err)
	require.True(t, progress.reserve("terminal"))
	progress.complete("terminal")
	require.NoError(t, progress.save())

	restored, err := newTerminalProgress(progressPath, "workflows-1.1-")
	require.NoError(t, err)
	require.False(t, restored.reserve("terminal"))

	otherGeneration, err := newTerminalProgress(progressPath, "workflows-1.2-")
	require.NoError(t, err)
	require.True(t, otherGeneration.reserve("terminal"))
}

func TestIndexBatchesCompletesTerminalProgressOnlyAfterSuccessfulWrite(t *testing.T) {
	t.Parallel()
	terminal := terminalWorkflow(t)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		controller := gomock.NewController(t)
		repository := mocks.NewMockWorkflowRepository(controller)
		progress, err := newTerminalProgress(filepath.Join(t.TempDir(), "progress.gob"), "workflows-1.1-")
		require.NoError(t, err)
		require.True(t, progress.reserve(terminal.ID))
		repository.EXPECT().CreateIndex(gomock.Any(), "workflows-1.1-2026-09-10").Return(nil)
		repository.EXPECT().Add(gomock.Any(), "workflows-1.1-2026-09-10", []*models.Workflow{terminal}).Return(nil)

		svc := &Service{repository: repository, logger: slog.New(slog.DiscardHandler), progress: progress}
		batches := make(chan workflowBatch, 1)
		batches <- workflowBatch{index: "workflows-1.1-2026-09-10", workflows: []*models.Workflow{terminal}}
		close(batches)
		svc.indexBatches(t.Context(), batches)

		require.False(t, progress.reserve(terminal.ID))
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		controller := gomock.NewController(t)
		repository := mocks.NewMockWorkflowRepository(controller)
		progress, err := newTerminalProgress(filepath.Join(t.TempDir(), "progress.gob"), "workflows-1.1-")
		require.NoError(t, err)
		require.True(t, progress.reserve(terminal.ID))
		repository.EXPECT().CreateIndex(gomock.Any(), "workflows-1.1-2026-09-10").Return(nil)
		repository.EXPECT().
			Add(gomock.Any(), "workflows-1.1-2026-09-10", []*models.Workflow{terminal}).
			Return(errors.New("unavailable"))

		svc := &Service{repository: repository, logger: slog.New(slog.DiscardHandler), progress: progress}
		batches := make(chan workflowBatch, 1)
		batches <- workflowBatch{index: "workflows-1.1-2026-09-10", workflows: []*models.Workflow{terminal}}
		close(batches)
		svc.indexBatches(t.Context(), batches)

		require.True(t, progress.reserve(terminal.ID))
	})
}

func TestBatchWorkflowsCoalescesExecutionSnapshots(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	open := &models.Workflow{ID: "workflow", Metadata: models.WorkflowMetadata{StartTime: start}, IndexVersion: 14}
	terminal := &models.Workflow{
		ID:           "workflow",
		Metadata:     models.WorkflowMetadata{StartTime: start, EndTime: &end},
		IndexVersion: 15,
	}
	input := make(chan *models.Workflow, 2)
	input <- open
	input <- terminal
	close(input)
	batches := make(chan workflowBatch, 1)

	(&Service{indexBatchSize: 100}).batchWorkflows(context.Background(), input, batches)
	batch := <-batches
	require.Equal(t, []*models.Workflow{terminal}, batch.workflows)
}

func terminalWorkflow(t *testing.T) *models.Workflow {
	t.Helper()
	start := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	return &models.Workflow{ID: "workflow", Metadata: models.WorkflowMetadata{StartTime: start, EndTime: &end}}
}
