package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/egoist/mygo"
)

const modelRecycleTestArgument = "onesub-test-recycle-model"

type modelRecycleRecipe struct {
	Model models.Model `json:"model"`
	Dir   string       `json:"dir"`
}

// A test-only child runs the real public MyGo shell API with its required
// native event loop, but creates no window and takes no desktop input.
func runModelRecycleTestChild(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var recipe modelRecycleRecipe
	if err := json.Unmarshal(raw, &recipe); err != nil {
		return err
	}
	var result error
	mygo.App.WhenReady(func() {
		result = recycleModelAssets(context.Background(), recipe.Model, recipe.Dir)
		mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return result
}
