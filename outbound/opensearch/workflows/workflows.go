// Package workflows implements the OpenSearch adapter for the workflows domain.
package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/opensearch-project/opensearch-go/v5"
	"github.com/opensearch-project/opensearch-go/v5/opensearchapi"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	os "github.com/varunbpatil/temporal-lens/outbound/opensearch"
	"github.com/varunbpatil/temporal-lens/types"
)

var _ ports.WorkflowRepository = (*Repository)(nil)

type Repository struct {
	client *opensearchapi.Client
	schema types.Schema
}

type WorkflowRepositoryParams struct {
	Config config.OpenSearchConfig
	Schema types.Schema
}

func NewRepository(_ context.Context, params WorkflowRepositoryParams) (*Repository, error) {
	osConfig := opensearch.Config{
		Addresses:          params.Config.Addresses,
		Username:           params.Config.Username,
		Password:           params.Config.Password,
		InsecureSkipVerify: params.Config.InsecureSkipVerify,
	}

	if params.Config.APIKey != "" {
		osConfig.Header = http.Header{}
		osConfig.Header.Set("Authorization", "ApiKey "+params.Config.APIKey)
	}

	client, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: osConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch: creating client: %w", err)
	}

	return &Repository{
		client: client,
		schema: params.Schema,
	}, nil
}

// CreateIndex creates an index with the full explicit mapping.
func (r *Repository) CreateIndex(ctx context.Context, index string) error {
	mapping := os.BuildIndexMapping(r.schema, []string{"activities", "childWorkflows"})

	body, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("opensearch: marshaling mapping: %w", err)
	}

	resp, err := r.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index:      index,
		BodyReader: bytes.NewReader(body),
	})
	if err != nil {
		target, ok := errors.AsType[*opensearch.StructError](err)
		if ok && target.Err.Type == "resource_already_exists_exception" {
			return ports.ErrIndexAlreadyExists
		}
		return fmt.Errorf("opensearch: creating index %q: %w", index, err)
	}
	if !resp.Acknowledged {
		return fmt.Errorf("opensearch: creating index %q: not acknowledged", index)
	}

	return nil
}

// DeleteIndex deletes an existing index. Returns nil if the index does not exist.
func (r *Repository) DeleteIndex(ctx context.Context, index string) error {
	resp, err := r.client.Indices.Delete(ctx, &opensearchapi.IndicesDeleteReq{
		Indices: []string{index},
		Params:  &opensearchapi.IndicesDeleteParams{IgnoreUnavailable: new(true)},
	})
	if err != nil {
		return fmt.Errorf("opensearch: deleting index %q: %w", index, err)
	}
	if !resp.Acknowledged {
		return fmt.Errorf("opensearch: deleting index %q: not acknowledged", index)
	}

	return nil
}

// ListIndexes lists all workflow indexes.
func (r *Repository) ListIndexes(ctx context.Context) ([]string, error) {
	resp, err := r.client.Cat.Indices(ctx, &opensearchapi.CatIndicesReq{})
	if err != nil {
		return nil, fmt.Errorf("opensearch: listing indexes: %w", err)
	}

	indexes := make([]string, 0, len(resp.Records))
	for _, rec := range resp.Records {
		if rec.Index != nil {
			indexes = append(indexes, *rec.Index)
		}
	}

	return indexes, nil
}

// Add indexes workflows using the bulk API.
// This call blocks until OpenSearch acknowledges the entire batch.
// Documents become searchable after the next refresh (default: 1s).
func (r *Repository) Add(ctx context.Context, index string, workflows []*models.Workflow) error {
	if len(workflows) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, w := range workflows {
		meta, err := json.Marshal(map[string]any{
			"index": map[string]any{
				"_index": index,
				"_id":    w.ID,
			},
		})
		if err != nil {
			return fmt.Errorf("opensearch: marshaling bulk meta: %w", err)
		}
		buf.Write(meta)
		buf.WriteByte('\n')

		src, err := json.Marshal(w)
		if err != nil {
			return fmt.Errorf("opensearch: marshaling document: %w", err)
		}
		buf.Write(src)
		buf.WriteByte('\n')
	}

	resp, err := r.client.Bulk(ctx, opensearchapi.BulkReq{
		Body: &buf,
	})
	if err != nil {
		return fmt.Errorf("opensearch: bulk indexing: %w", err)
	}
	if resp.Errors {
		return fmt.Errorf("opensearch: bulk indexing had errors")
	}

	return nil
}

// Search searches for workflows matching the filter, with sort and pagination.
func (r *Repository) Search(ctx context.Context, index string, req ports.SearchRequest) (ports.SearchResponse, error) {
	query, err := buildSearchBody(req)
	if err != nil {
		return ports.SearchResponse{}, fmt.Errorf("opensearch: building query: %w", err)
	}

	body, err := json.Marshal(query)
	if err != nil {
		return ports.SearchResponse{}, fmt.Errorf("opensearch: marshaling query: %w", err)
	}

	resp, err := r.client.Search(ctx, &opensearchapi.SearchReq{
		Indices:    []string{index},
		BodyReader: bytes.NewReader(body),
	})
	if err != nil {
		return ports.SearchResponse{}, fmt.Errorf("opensearch: searching: %w", err)
	}

	workflows := make([]*models.Workflow, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var w models.Workflow
		if unmarshalErr := json.Unmarshal(hit.Source, &w); unmarshalErr != nil {
			return ports.SearchResponse{}, fmt.Errorf("opensearch: unmarshaling hit: %w", unmarshalErr)
		}
		workflows = append(workflows, &w)
	}

	var totalHits int64
	if resp.Hits.Total != nil {
		totalHits, _ = resp.Hits.Total.Int64()
	}

	return ports.SearchResponse{
		Workflows: workflows,
		TotalHits: totalHits,
		Took:      time.Duration(resp.Took) * time.Millisecond,
	}, nil
}

func buildSearchBody(req ports.SearchRequest) (map[string]any, error) {
	body := map[string]any{}

	query, err := os.BuildQuery(req.Filter)
	if err != nil {
		return nil, err
	}
	body["query"] = query
	body["track_total_hits"] = true

	if req.Sort != nil {
		order := "asc"
		if req.Sort.Order == types.SortOrderDesc {
			order = "desc"
		}
		body["sort"] = []map[string]any{
			{req.Sort.Field: map[string]any{"order": order}},
		}
	}

	if req.Pagination != nil {
		if pagErr := applyPagination(body, req.Pagination); pagErr != nil {
			return nil, pagErr
		}
	}

	return body, nil
}

func applyPagination(body map[string]any, p *types.Pagination) error {
	if p.Offset != nil {
		offset := int((p.Offset.PageNumber - 1) * p.Offset.PageSize)
		body["from"] = offset
		body["size"] = int(p.Offset.PageSize)
		return nil
	}

	if p.Cursor != nil {
		body["size"] = int(p.Cursor.PageSize)
		if p.Cursor.Cursor != "" {
			var searchAfter []any
			if err := json.Unmarshal([]byte(p.Cursor.Cursor), &searchAfter); err != nil {
				return fmt.Errorf("opensearch: decoding cursor: %w", err)
			}
			body["search_after"] = searchAfter
		}
	}

	return nil
}
