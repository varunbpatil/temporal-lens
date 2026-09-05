package ports

import (
	"time"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/types"
)

type SearchRequest struct {
	Filter     *types.Filter
	Sort       *types.Sort
	Pagination *types.Pagination
}

type SearchResponse struct {
	Workflows []*models.Workflow
	TotalHits int64
	Took      time.Duration
}

type SignalRequest struct{}

type ResetRequest struct{}

type TerminateRequest struct{}

type StreamWorkflowMetadataRequest struct{}

type StreamWorkflowDataRequest struct{}

type SourceSignalRequest struct{}

type SourceResetRequest struct{}

type SourceTerminateRequest struct{}
