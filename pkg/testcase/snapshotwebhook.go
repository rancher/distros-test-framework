package testcase

import (
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

func TestSnapshotWebhook(applyWorkload bool) error {
	assert := " Spec.VolumeSnapshotClassName must not be the empty string"

	if applyWorkload {
		workloadErr := shared.ManageWorkload("apply", "snapshot-webhook.yaml")
		if workloadErr != nil {
			if strings.Contains(workloadErr.Error(), assert) {
				shared.LogLevel("error", workloadErr.Error())
				shared.LogLevel("info", "Snapshot Webhook manifest not deployed, "+
					"as expected related to empty string")

				return workloadErr
			}
			shared.LogLevel("error", workloadErr.Error(),
				"Error: webhook not rejecting bad VolumeSnapshot, please double check")

			return nil
		}
	}

	return nil
}
