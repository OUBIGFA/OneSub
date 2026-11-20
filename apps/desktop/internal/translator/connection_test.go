package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestModelConnectionUsesTheSelectedProtocolAndCredential(t *testing.T) {
	for _, api := range []string{"", APIChatCompletions, APIResponses} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Keep the measured round trip above the Windows clock tick;
					// an immediate loopback reply can otherwise take zero ticks.
					time.Sleep(time.Millisecond)
					calls++
					path := "/v1/chat/completions"
					if api == APIResponses {
						path = "/v1/responses"
					}
					if r.Method != http.MethodPost || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer local-test" {
						t.Error("probe changed the service, protocol or credential", r.Method, r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != "model-b" || body["stream"] != true {
						t.Error("probe did not request the selected model using the translation wire mode", err, body)
					}
					if api == APIResponses {
						if body["store"] != false || body["input"] == nil {
							t.Error("Responses probe lost its input or privacy setting", body)
						}
					} else if body["messages"] == nil {
						t.Error("Chat Completions probe has no prompt", body)
					}
					if api == APIResponses && stream {
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n")
					} else if api == APIResponses {
						fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}`)
					} else if stream {
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					} else {
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`)
					}
				}))
				defer server.Close()
				elapsed, err := CheckModel(context.Background(), server.URL+"/v1/", api, "model-b", "local-test", server.Client())
				if err != nil || elapsed <= 0 || calls != 1 {
					t.Fatal("complete model response did not pass", elapsed, calls, err)
				}
			})
		}
	}
}

// HTTP success and the first output chunk are insufficient evidence. The
// shared parser must finish the response before the model is shown as usable.
func TestModelConnectionRejectsEmptyAndInterruptedResponses(t *testing.T) {
	for _, tc := range []struct{ name, api, body string }{
		{"empty", APIChatCompletions, `{"choices":[{"message":{"content":"  "},"finish_reason":"stop"}]}`},
		{"refusal", APIChatCompletions, `{"choices":[{"message":{"refusal":"unavailable"},"finish_reason":"stop"}]}`},
		{"filtered", APIChatCompletions, `{"choices":[{"message":{"content":"partial"},"finish_reason":"content_filter"}]}`},
		{"failed-chat", APIChatCompletions, `{"choices":[{"message":{"content":"partial"},"finish_reason":"error"}]}`},
		{"invalid-json", APIChatCompletions, `{`},
		{"cut-stream", APIChatCompletions, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"},
		{"late-stream-error", APIChatCompletions, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"error\":{\"message\":\"model unavailable\"}}\n\n"},
		{"empty-responses", APIResponses, `{"status":"completed","output":[]}`},
		{"incomplete-responses", APIResponses, `{"status":"incomplete","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`},
		{"failed-responses", APIResponses, `{"status":"failed","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}],"error":{"message":"model unavailable"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer server.Close()
			if _, err := CheckModel(context.Background(), server.URL, tc.api, "model-a", "", nil); err == nil {
				t.Fatal("unusable response passed the connection test")
			}
		})
	}
}

func TestModelConnectionDoesNotRedirectOrRetry(t *testing.T) {
	for _, status := range []int{302, 401, 403, 404, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/other")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"model unavailable"}}`)
			}))
			defer server.Close()
			if _, err := CheckModel(context.Background(), server.URL, APIChatCompletions, "model-a", "", server.Client()); err == nil || calls != 1 {
				t.Fatal("failed probe redirected, retried or passed", status, calls, err)
			}
		})
	}
}

func TestModelConnectionDeadlineCoversAnActiveStream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := CheckModel(ctx, server.URL, APIChatCompletions, "model-a", "", nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second || calls.Load() != 1 {
		t.Fatal("active stream escaped the total deadline", calls.Load(), err)
	}
}

func TestModelConnectionValidatesBeforeSending(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	for _, tc := range []struct{ endpoint, model string }{{server.URL, " "}, {server.URL + "?key=invalid", "model-a"}} {
		if _, err := CheckModel(context.Background(), tc.endpoint, "", tc.model, "", nil); err == nil {
			t.Fatal("invalid probe configuration accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckModel(ctx, server.URL, "", "model-a", "", nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled probe did not preserve cancellation", err)
	}
	if calls != 0 {
		t.Fatal("invalid or cancelled probe contacted the service", calls)
	}
}
