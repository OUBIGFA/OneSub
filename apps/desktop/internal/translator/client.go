package translator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// The request protocols of OpenAI-compatible services. Both are reached
// under the same base URL, which also lists the models.
const (
	APIChatCompletions = "chat-completions"
	APIResponses       = "responses"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// request is one model call: a phase's system prompt, the turns after it
// and, for a JSON answer, the schema its keys follow.
type request struct {
	system   string
	messages []message
	schema   map[string]any // nil: a plain-text answer
}

// reply is a model's complete answer.
type reply struct {
	text          string
	truncated     bool // cut by the output limit or a content filter
	input, output int
}

// streamed receives the answer so far and the end of the model's reasoning,
// as they arrive.
type streamed func(content, thinking string)

// Structured output is asked for in the strictest form a service accepts,
// stepping down as SmartSub's structuredOutputFallback does: a JSON schema
// fixing the batch's keys, then any JSON object, then a plain answer that
// the tolerant parser reads. A step down holds for the rest of the run.
const (
	modeSchema = iota
	modeObject
	modePlain
)

const (
	maxRetries = 3
	// idleTimeout ends a request that sends nothing at all for this long,
	// first byte included. Models that think before answering stay silent
	// for a while, so it is generous.
	idleTimeout = 4 * time.Minute
	maxAnswer   = 8 << 20
)

// service talks to the one OpenAI-compatible endpoint the user chose, for
// one run. Requests stream, so a reader sees the translation as it is
// written. Transient failures (rate limits, overloaded or unreachable
// services, cut streams) are retried a bounded number of times with
// backoff, as VideoCaptioner's and SmartSub's clients do; every attempt
// counts toward the run's request limit, which caps the cost.
type service struct {
	url, api, model, key string
	client               *http.Client
	maxRequests          int
	onRetry              func(note string)

	mu                      sync.Mutex
	mode                    int
	noUsage                 bool // the service rejected stream_options
	requests, input, output int
}

func newService(url, api, model, key string, client *http.Client, maxRequests int) *service {
	if client == nil {
		client = &http.Client{}
	}
	copied := *client
	// Never forward a POST to another address: its charge and outcome there
	// are unknown.
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// Streaming answers may take minutes; idleTimeout bounds silence instead.
	copied.Timeout = 0
	return &service{url: url, api: api, model: model, key: key, client: &copied, maxRequests: maxRequests}
}

// route is the address a request of the protocol is sent to.
func route(base, api string) string {
	if api == APIResponses {
		return base + "/responses"
	}
	return base + "/chat/completions"
}

type httpError struct {
	status     int
	detail     string
	retryAfter time.Duration
}

func (e *httpError) Error() string {
	switch e.status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return locales.ErrTranslateAuth(e.status, e.detail)
	case http.StatusNotFound:
		return locales.ErrTranslateNotFound(e.detail)
	}
	return locales.ErrTranslateHttp(e.status, e.detail)
}

// refusal is a model declining to answer; sending it again does not help.
type refusal struct{ reason string }

func (r refusal) Error() string { return locales.ErrTranslateRefused(r.reason) }

var errIdle = errors.New(locales.ErrTranslateIdle())

func (s *service) usage() (requests, input, output int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.input, s.output
}

// call sends r until it gets a complete answer, a permanent error, or runs
// out of retries or of the run's requests.
func (s *service) call(ctx context.Context, r request, stream streamed) (reply, error) {
	for attempt := 0; ; {
		s.mu.Lock()
		if s.maxRequests > 0 && s.requests >= s.maxRequests {
			s.mu.Unlock()
			return reply{}, errors.New(locales.ErrRequestLimit())
		}
		s.requests++
		mode, usage := s.mode, !s.noUsage
		s.mu.Unlock()
		if r.schema == nil {
			mode = modePlain
		}
		got, err := s.send(ctx, r, mode, usage, stream)
		if err == nil {
			s.mu.Lock()
			s.input += got.input
			s.output += got.output
			s.mu.Unlock()
			return got, nil
		}
		if ctx.Err() != nil {
			return reply{}, ctx.Err()
		}
		var status *httpError
		if errors.As(err, &status) && (status.status == http.StatusBadRequest || status.status == http.StatusUnprocessableEntity) {
			detail := strings.ToLower(status.detail)
			if usage && strings.Contains(detail, "stream_options") {
				s.mu.Lock()
				s.noUsage = true
				s.mu.Unlock()
				continue
			}
			// Any rejection of a schema steps down; a rejection of a JSON
			// object only when it names the response format.
			if mode == modeSchema || (mode == modeObject && mentionsFormat(detail)) {
				s.mu.Lock()
				s.mode = max(s.mode, mode+1)
				s.mu.Unlock()
				continue
			}
		}
		if !retryable(err) || attempt >= maxRetries {
			return reply{}, err
		}
		attempt++
		wait := backoff(attempt, status)
		if s.onRetry != nil {
			s.onRetry(locales.TranslationRetry(attempt, maxRetries, int(wait.Round(time.Second)/time.Second), err.Error()))
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return reply{}, ctx.Err()
		}
	}
}

func mentionsFormat(detail string) bool {
	for _, word := range []string{"response_format", "json_schema", "json_object", "structured", "json mode", "text.format"} {
		if strings.Contains(detail, word) {
			return true
		}
	}
	return false
}

// retryable: what may succeed when sent again. Credentials, a missing model,
// a request the service cannot take and a refusal do not.
func retryable(err error) bool {
	var status *httpError
	if errors.As(err, &status) {
		switch status.status {
		case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return status.status >= 520 && status.status <= 529
	}
	var refused refusal
	return !errors.As(err, &refused)
}

// backoff waits as long as the service asks, else 2, 5 and 10 seconds,
// with jitter so parallel batches do not return together.
func backoff(attempt int, status *httpError) time.Duration {
	if status != nil && status.retryAfter > 0 {
		return min(status.retryAfter, time.Minute)
	}
	steps := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}
	return steps[min(attempt, len(steps))-1] + rand.N(time.Second)
}

func (s *service) body(r request, mode int, usage bool) map[string]any {
	format := map[string]any{"type": "json_schema", "name": "subtitles", "strict": true, "schema": r.schema}
	if s.api == APIResponses {
		input := []message{}
		input = append(input, r.messages...)
		// Each request stands alone, so the service is asked not to store it.
		body := map[string]any{"model": s.model, "instructions": r.system, "input": input, "stream": true, "store": false}
		switch mode {
		case modeSchema:
			body["text"] = map[string]any{"format": format}
		case modeObject:
			body["text"] = map[string]any{"format": map[string]any{"type": "json_object"}}
		}
		return body
	}
	messages := append([]message{{"system", r.system}}, r.messages...)
	body := map[string]any{"model": s.model, "messages": messages, "stream": true}
	if usage {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	switch mode {
	case modeSchema:
		delete(format, "type")
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": format}
	case modeObject:
		body["response_format"] = map[string]any{"type": "json_object"}
	}
	return body
}

func (s *service) send(ctx context.Context, r request, mode int, usage bool, stream streamed) (reply, error) {
	payload, err := json.Marshal(s.body(r, mode, usage))
	if err != nil {
		return reply{}, err
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(idleTimeout, cancel)
	defer idle.Stop()
	timedOut := func(err error) error {
		if ctx.Err() == nil && requestCtx.Err() != nil {
			return apperr.Wrap(errIdle.Error(), err)
		}
		return err
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return reply{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if s.key != "" {
		req.Header.Set("Authorization", "Bearer "+s.key)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return reply{}, timedOut(apperr.Wrap(locales.ErrTranslateNetwork(err.Error()), err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return reply{}, &httpError{status: response.StatusCode, detail: errorDetail(raw), retryAfter: retryAfter(response.Header.Get("Retry-After"))}
	}
	got, err := s.read(response.Body, func() { idle.Reset(idleTimeout) }, stream)
	if err != nil {
		return reply{}, timedOut(err)
	}
	return got, nil
}

// answer collects one streamed reply.
type answer struct {
	content, thinking strings.Builder
	refused           strings.Builder
	finish            string
	input, output     int
}

// read takes Server-Sent Events, or the one JSON body a service that
// ignores "stream" sends.
func (s *service) read(body io.Reader, alive func(), stream streamed) (reply, error) {
	reader := bufio.NewReaderSize(body, 64<<10)
	var a answer
	started := false
	done := false
	for !done {
		line, err := reader.ReadString('\n')
		alive()
		text := strings.TrimSpace(line)
		if !started && strings.HasPrefix(text, "{") {
			rest, _ := io.ReadAll(io.LimitReader(reader, maxAnswer))
			return s.whole(append([]byte(line), rest...))
		}
		started = started || text != ""
		if data, ok := strings.CutPrefix(text, "data:"); ok {
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				break
			}
			finished, eventErr := s.event([]byte(data), &a)
			if eventErr != nil {
				return reply{}, eventErr
			}
			done = finished
			if a.content.Len()+a.thinking.Len() > maxAnswer {
				return reply{}, errors.New(locales.ErrTranslateSize())
			}
			if stream != nil {
				stream(a.content.String(), tail(a.thinking.String(), 160))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return reply{}, apperr.Wrap(locales.ErrTranslateNetwork(err.Error()), err)
		}
	}
	if a.finish == "" {
		// The stream ended without saying how: cut on the way.
		return reply{}, errors.New(locales.ErrOutcomeInterrupted())
	}
	return a.reply()
}

func (a *answer) reply() (reply, error) {
	if a.content.Len() == 0 && a.refused.Len() > 0 {
		return reply{}, refusal{a.refused.String()}
	}
	cut := a.finish == "length" || a.finish == "content_filter" || a.finish == "incomplete" || a.finish == "max_tokens"
	return reply{text: a.content.String(), truncated: cut, input: a.input, output: a.output}, nil
}

// event reads one SSE data payload. It reports when the reply is complete.
func (s *service) event(data []byte, a *answer) (bool, error) {
	if s.api == APIResponses {
		var ev struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Message  string `json:"message"`
			Response *struct {
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
		}
		if json.Unmarshal(data, &ev) != nil {
			return false, nil
		}
		usage := func() {
			if ev.Response != nil && ev.Response.Usage != nil {
				a.input, a.output = ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens
			}
		}
		switch ev.Type {
		case "response.output_text.delta":
			a.content.WriteString(ev.Delta)
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			a.thinking.WriteString(ev.Delta)
		case "response.refusal.delta":
			a.refused.WriteString(ev.Delta)
		case "response.completed":
			a.finish = "stop"
			usage()
			return true, nil
		case "response.incomplete":
			a.finish = "incomplete"
			usage()
			return true, nil
		case "response.failed":
			message := "response.failed"
			if ev.Response != nil && ev.Response.Error != nil {
				message = ev.Response.Error.Message
			}
			return false, errors.New(locales.ErrTranslateStream(message))
		case "error":
			return false, errors.New(locales.ErrTranslateStream(ev.Message))
		}
		return false, nil
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          *string `json:"content"`
				ReasoningContent *string `json:"reasoning_content"`
				Reasoning        *string `json:"reasoning"`
				Refusal          *string `json:"refusal"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &chunk) != nil {
		return false, nil
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return false, errors.New(locales.ErrTranslateStream(errorDetail(data)))
	}
	if chunk.Usage != nil && chunk.Usage.PromptTokens+chunk.Usage.CompletionTokens > 0 {
		a.input, a.output = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
	}
	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		if choice.Delta.Content != nil {
			a.content.WriteString(*choice.Delta.Content)
		}
		// DeepSeek and LM Studio name streamed reasoning reasoning_content,
		// OpenRouter reasoning.
		for _, part := range []*string{choice.Delta.ReasoningContent, choice.Delta.Reasoning} {
			if part != nil {
				a.thinking.WriteString(*part)
			}
		}
		if choice.Delta.Refusal != nil {
			a.refused.WriteString(*choice.Delta.Refusal)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			a.finish = *choice.FinishReason
		}
	}
	// The usage chunk follows the finishing one; [DONE] ends the stream.
	return false, nil
}

// whole reads a complete, non-streamed reply.
func (s *service) whole(raw []byte) (reply, error) {
	var a answer
	if s.api == APIResponses {
		var wire struct {
			Status string `json:"status"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type    string `json:"type"`
					Text    string `json:"text"`
					Refusal string `json:"refusal"`
				} `json:"content"`
			} `json:"output"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return reply{}, errors.New(locales.ErrTranslateJson())
		}
		for _, item := range wire.Output {
			// Reasoning and other items carry no part of the answer.
			if item.Type != "message" {
				continue
			}
			for _, part := range item.Content {
				a.content.WriteString(part.Text)
				a.refused.WriteString(part.Refusal)
			}
		}
		a.finish = map[string]string{"completed": "stop", "incomplete": "incomplete"}[wire.Status]
		a.input, a.output = wire.Usage.InputTokens, wire.Usage.OutputTokens
	} else {
		var wire struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return reply{}, errors.New(locales.ErrTranslateJson())
		}
		if len(wire.Choices) == 0 {
			return reply{}, errors.New(locales.ErrTranslateStream(errorDetail(raw)))
		}
		a.content.WriteString(wire.Choices[0].Message.Content)
		a.refused.WriteString(wire.Choices[0].Message.Refusal)
		a.finish = wire.Choices[0].FinishReason
		a.input, a.output = wire.Usage.PromptTokens, wire.Usage.CompletionTokens
	}
	if a.finish == "" {
		a.finish = "stop"
	}
	return a.reply()
}

// errorDetail is a service's own error message, short enough for a status
// bar; services put it in error.message, error, message or detail.
func errorDetail(raw []byte) string {
	var wire struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Detail  any             `json:"detail"`
	}
	text := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &wire) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var plain string
		switch {
		case json.Unmarshal(wire.Error, &nested) == nil && nested.Message != "":
			text = nested.Message
		case json.Unmarshal(wire.Error, &plain) == nil && plain != "":
			text = plain
		case wire.Message != "":
			text = wire.Message
		case wire.Detail != nil:
			if detail, err := json.Marshal(wire.Detail); err == nil {
				text = strings.Trim(string(detail), `"`)
			}
		}
	}
	return tail(strings.Join(strings.Fields(text), " "), -300)
}

// tail keeps the last n runes of text (the first -n when n < 0), on one line.
func tail(text string, n int) string {
	if n > 0 && len(text) > 8*n {
		// Only the end is shown; do not re-read a long reasoning each time.
		text = text[len(text)-8*n:]
		for len(text) > 0 && !utf8.RuneStart(text[0]) {
			text = text[1:]
		}
	}
	runes := []rune(strings.Join(strings.Fields(text), " "))
	switch {
	case n > 0 && len(runes) > n:
		return "…" + string(runes[len(runes)-n:])
	case n < 0 && len(runes) > -n:
		return string(runes[:-n]) + "…"
	}
	return string(runes)
}

func retryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return time.Until(at)
	}
	return 0
}
