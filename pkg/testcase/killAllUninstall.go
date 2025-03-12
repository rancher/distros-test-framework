package testcase

import (
	"github.com/rancher/distros-test-framework/shared"
)

func TestKillAll(cluster *shared.Cluster) {

	// first test with previously data dir mounted
	// scp kill all test script to the node
	// and have a way to return back to here the results of that script

	// validate kill all stuff

	// install product again and test with no data dir mounted
}

func TestUninstall(cluster *shared.Cluster) {

	// scp uninstall test script to the node

	// and have a way to return back to here the results of that script

	// validate kill all stuff
	// validate uninstall stuff
}
