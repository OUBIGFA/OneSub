// Provider model discovery is read-only and independent of translation billing.
package translator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Models queries only the explicitly configured service, without redirects or
// automatic retries. Unsupported discovery never disables manual model entry.
func Models(ctx context.Context, endpointURL, key string, client *http.Client) ([]string, error) {
	api, err := endpoint(endpointURL)
	if err != nil {
		return nil, err
	}
	api = strings.TrimSuffix(api, "/chat/completions") + "/models"
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
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, errors.New(locales.ErrModelsFormat())
	}
	names := []string{}
	for _, model := range result.Data {
		if name := strings.TrimSpace(model.ID); name != "" && len(name) <= 1024 {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)
	if len(names) == 0 {
		return nil, errors.New(locales.ErrModelsEmpty())
	}
	return names, nil
}
