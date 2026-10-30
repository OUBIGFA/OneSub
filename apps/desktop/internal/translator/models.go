// Provider model discovery is read-only and independent of translation billing.
package translator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// ModelInfo contains only limits advertised by this endpoint or entered from
// its documentation. Zero means unknown, never a guessed model-family default.
type ModelInfo struct {
	ID              string `json:"id"`
	MaxInputTokens  int    `json:"maxInputTokens,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
	ContextWindow   int    `json:"contextWindow,omitempty"`
	LimitsSource    string `json:"limitsSource,omitempty"`
}

// Models queries only the explicitly configured service, without redirects or
// automatic retries. Unsupported discovery never disables manual model entry.
func Models(ctx context.Context, endpointURL, key string, client *http.Client) ([]string, error) {
	items, err := ModelCatalog(ctx, endpointURL, key, client)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.ID)
	}
	return names, nil
}

// ModelCatalog preserves the endpoint's advertised token limits. The standard
// /models response may contain IDs only; those limits stay unknown.
func ModelCatalog(ctx context.Context, endpointURL, key string, client *http.Client) ([]ModelInfo, error) {
	api, err := endpoint(endpointURL)
	if err != nil {
		return nil, err
	}
	api += "/models"
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(req)
	if err != nil {
		return nil, apperr.Wrap(locales.ErrModelsFetch(err.Error()), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New(locales.ErrModelsHttp(response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2*1024*1024 {
		return nil, errors.New(locales.ErrModelsSize())
	}
	var result struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, errors.New(locales.ErrModelsFormat())
	}
	items := map[string]ModelInfo{}
	for _, model := range result.Data {
		var id string
		if json.Unmarshal(model["id"], &id) != nil {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 1024 {
			continue
		}
		info := ModelInfo{ID: id, MaxInputTokens: declaredLimit(model, "max_input_tokens", "input_token_limit"), MaxOutputTokens: declaredLimit(model, "max_output_tokens", "max_completion_tokens", "output_token_limit"), ContextWindow: declaredLimit(model, "context_length", "context_window"), LimitsSource: api}
		var top map[string]json.RawMessage
		if json.Unmarshal(model["top_provider"], &top) == nil {
			if info.ContextWindow == 0 {
				info.ContextWindow = declaredLimit(top, "context_length", "context_window")
			}
			if info.MaxOutputTokens == 0 {
				info.MaxOutputTokens = declaredLimit(top, "max_completion_tokens", "max_output_tokens")
			}
		}
		items[id] = info
	}
	models := make([]ModelInfo, 0, len(items))
	for _, info := range items {
		models = append(models, info)
	}
	slices.SortFunc(models, func(a, b ModelInfo) int { return strings.Compare(a.ID, b.ID) })
	if len(models) == 0 {
		return nil, errors.New(locales.ErrModelsEmpty())
	}
	return models, nil
}

func declaredLimit(fields map[string]json.RawMessage, names ...string) int {
	for _, name := range names {
		value := strings.Trim(string(fields[name]), "\"")
		if n, err := strconv.ParseInt(value, 10, 32); err == nil && n > 0 {
			return int(n)
		}
	}
	return 0
}
