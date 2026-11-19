package translator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// CheckModel probes the selected model through the translation transport.
// Like Cherry Studio's model check, it consumes a complete short answer and
// bounds the whole request. No subtitle, history, retry or redirect is involved.
func CheckModel(ctx context.Context, endpointURL, api, model, key string, client *http.Client) (time.Duration, error) {
	base, err := endpoint(endpointURL)
	if err != nil {
		return 0, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return 0, errors.New(locales.ErrTranslateModel())
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	service := newService(route(base, api), api, model, key, client, 1)
	started := time.Now()
	// send is one attempt. Optional usage/schema parameters are unnecessary
	// for a plain-text probe and can be rejected by compatible services.
	got, err := service.send(ctx, request{system: "Reply with OK.", messages: []message{{Role: "user", Content: "Hello."}}}, modePlain, false, nil)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return 0, apperr.Wrap(locales.TranslationModelTestTimeout(), ctx.Err())
	}
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(stripThinking(got.text)) == "" || got.finish != "stop" {
		return 0, errors.New(locales.TranslationModelTestEmpty())
	}
	return time.Since(started), nil
}
