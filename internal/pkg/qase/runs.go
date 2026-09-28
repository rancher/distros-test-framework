package qase

import (
	"context"
	"errors"
	"fmt"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"
)

// SearchRuns lists up to 100 runs of the project whose title matches search.
// ctx controls cancellation; authentication comes from the client.
func (c Client) SearchRuns(ctx context.Context, search string) ([]qaseclient.Run, error) {
	authCtx := context.WithValue(ctx, qaseclient.ContextAPIKeys, c.Ctx.Value(qaseclient.ContextAPIKeys))

	list, res, err := c.QaseAPI.RunsAPI.GetRuns(authCtx, projectID).Search(search).Limit(100).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to search runs: %w, response: %v", err, res)
	}
	if list == nil || !list.GetStatus() {
		return nil, errors.New("failed to search runs: status false")
	}

	return list.GetResult().Entities, nil
}
