include ./config/.env

#========================= Run acceptance tests in docker =========================#

test-env-up:
	@./scripts/docker_run.sh test-env-up

test-run:
	@./scripts/docker_run.sh test-run

## Use this to run automatically without need to change image name
test-run-new:
	@./scripts/docker_run.sh test-run-new

## Use this to build and run automatically
test-build-run:
	@./scripts/docker_run.sh test-build-run

## Use this to run on the same environment + cluster from the previous last container -${TAGNAME} created
test-run-state:
	@./scripts/docker_run.sh test-run-state

## Use this to run code changes on the same cluster from the previous run. Useful for debugging new code.
test-run-updates:
	@./scripts/docker_run.sh test-run-updates

## use this to test a new run on a totally new fresh environment after delete also aws resources
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run

test-logs:
	@./scripts/docker_run.sh test-logs

image-stats:
	@./scripts/docker_run.sh image-stats

test-env-down:
	@./scripts/docker_run.sh test-env-down

test-env-clean:
	@./scripts/delete_resources.sh

#========================= Run acceptance tests locally =========================#
remove-tf-state:
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform.lock.hcl ./modules/${ENV_PRODUCT}/terraform.tfstate ./modules/${ENV_PRODUCT}/terraform.tfstate.backup

## use this to skip tests
test-skip:
	ifdef SKIP
		SKIP_FLAG=--ginkgo.skip="${SKIP}"
	endif


test-create:
	@go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...


test-cert-rotate:
	@go test -timeout=45m -v -count=1 ./entrypoint/certrotate/...


test-secrets-encrypt:
	@go test -timeout=45m -v -count=1 ./entrypoint/secretsencrypt/...


test-validate:
	@go test -timeout=45m -v -count=1 ./entrypoint/validatecluster/...


test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion ${SUC_UPGRADE_VERSION} -channel "${CHANNEL}"


test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT} -channel ${CHANNEL}


test-upgrade-node-replacement:
	@go test -timeout=120m -v -tags=upgradereplacement -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT} -channel ${CHANNEL}


test-create-mixedos:
	@go test -timeout=45m -v -count=1 ./entrypoint/mixedoscluster/... $(if ${SONOBUOY_VERSION},-sonobuoyVersion ${SONOBUOY_VERSION})


test-create-dualstack:
	@go test -timeout=45m -v -count=1 ./entrypoint/dualstack/...


test-version-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=versionbump \
	-cmd "${CMD}" \
    -expectedValue ${EXPECTED_VALUE} \
    $(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DESCRIPTION},-description "${DESCRIPTION}") \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


test-components-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=components \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT})


test-validate-selinux:
	@go test -timeout=45m -v -count=1 ./entrypoint/selinux/... \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL})


test-restart-service:
	@go test -timeout=45m -v -count=1 ./entrypoint/restartservice/...

test-reboot-instances:
	@go test -timeout=45m -v -count=1 ./entrypoint/rebootinstances/...



#========================= TestCode Static Quality Check =========================#
pre-commit:
	@gofmt -s -w .
	@goimports -w .
	@go vet ./...
	@golangci-lint run --tests ./...