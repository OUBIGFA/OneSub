package translator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestProviderModelsPreservesEndpointKeyAndIDs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/models" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer local-test" {
			t.Error(r.URL, r.Header)
		}
		fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":"a-model"},{"id":"a-model"},{"id":""}]}`)
	}))
	defer server.Close()
	models, err := Models(context.Background(), server.URL+"/v1/", "local-test", nil)
	if err != nil || !reflect.DeepEqual(models, []string{"a-model", "z-model"}) || calls != 1 {
		t.Fatal(models, calls, err)
	}
}
func TestProviderModelErrorsDoNotRedirectOrRetry(t *testing.T) {
	for _, status := range []int{302, 401, 404, 429, 500} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Location", "/other")
			w.WriteHeader(status)
		}))
		_, err := Models(context.Background(), server.URL, "", nil)
		server.Close()
		if err == nil || calls != 1 {
			t.Fatal(status, calls, err)
		}
	}
}

func TestModelLimitsComeFromTheConfiguredProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"a","max_input_tokens":96000,"max_output_tokens":12000,"context_window":100000},{"id":"b","top_provider":{"context_length":32000,"max_completion_tokens":4096}},{"id":"c"}]}`)
	}))
	defer server.Close()
	models, err := ModelCatalog(context.Background(), server.URL, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].MaxInputTokens != 96000 || models[0].MaxOutputTokens != 12000 || models[0].ContextWindow != 100000 || models[1].MaxInputTokens != 0 || models[1].MaxOutputTokens != 4096 || models[2].ContextWindow != 0 || models[0].LimitsSource != server.URL+"/models" {
		t.Fatal(models)
	}
}
