package qase

import (
	"context"
	"errors"
	"os"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"
)

type Client struct {
	QaseAPI *qaseclient.APIClient
	Ctx     context.Context
}

func AddQase() (*Client, error) {
	qaseToken := os.Getenv("QASE_AUTOMATION_TOKEN")
	if qaseToken == "" {
		return nil, errors.New("QASE_AUTOMATION_TOKEN is not set")
	}

	ctx := context.WithValue(context.Background(), qaseclient.ContextAPIKeys, map[string]qaseclient.APIKey{
		"TokenAuth": {
			Key: qaseToken,
		},
	})

	return &Client{
		QaseAPI: qaseclient.NewAPIClient(qaseclient.NewConfiguration()),
		Ctx:     ctx,
	}, nil
}
