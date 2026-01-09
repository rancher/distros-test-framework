package qase

import (
	"context"
	"errors"
	"os"
	"time"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"
)

const (
	// QaseAPITimeout is the maximum time to wait for Qase API responses
	QaseAPITimeout = 30 * time.Second
)

type Client struct {
	QaseAPI *qaseclient.APIClient
	Ctx     context.Context
	Cancel  context.CancelFunc
}

func AddQase() (*Client, error) {
	qaseToken := os.Getenv("QASE_AUTOMATION_TOKEN")
	if qaseToken == "" {
		return nil, errors.New("QASE_AUTOMATION_TOKEN is not set")
	}

	// Create context with timeout to prevent hanging on API calls
	ctx, cancel := context.WithTimeout(context.Background(), QaseAPITimeout)
	
	ctx = context.WithValue(ctx, qaseclient.ContextAPIKeys, map[string]qaseclient.APIKey{
		"TokenAuth": {
			Key: qaseToken,
		},
	})

	return &Client{
		QaseAPI: qaseclient.NewAPIClient(qaseclient.NewConfiguration()),
		Ctx:     ctx,
		Cancel:  cancel,
	}, nil
}
