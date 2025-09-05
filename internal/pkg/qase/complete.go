package qase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/resources"
)

func (c Client) completeRun(runID int32) error {
	baseRes, res, err := c.QaseAPI.RunsAPI.CompleteRun(c.Ctx, projectID, runID).Execute()
	if err != nil {
		return fmt.Errorf("failed to complete run: %w, response: %v", err, res)
	}

	resources.LogLevel("debug", "Run completed: %v\n", &baseRes)

	return nil
}
