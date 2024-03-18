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

## Use this to run on the same environement + cluster from the previous last container -${TAGNAME} created
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

.PHONY: test-env-down
test-env-down:
	@./scripts/docker_run.sh test-env-down

.PHONY: test-env-clean
test-env-clean:
	@./scripts/delete_resources.sh


#========================= Run acceptance tests locally =========================#
.PHONY: remove-tf-state
remove-tf-state:
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform.lock.hcl ./modules/${ENV_PRODUCT}/terraform.tfstate ./modules/${ENV_PRODUCT}/terraform.tfstate.backup

## use this to skip tests
test-skip:
	ifdef SKIP
		SKIP_FLAG=--ginkgo.skip="${SKIP}"
	endif

.PHONY: test-create
test-create:
	@go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...

.PHONY: test-cert-rotate
test-cert-rotate:
	@go test -timeout=45m -v -count=1 ./entrypoint/certrotate/...


.PHONY: test-validate
test-validate:
	@go test -timeout=45m -v -count=1 ./entrypoint/validatecluster/...


.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion ${SUC_UPGRADE_VERSION}

.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT} -channel ${CHANNEL}

.PHONY: test-upgrade-node-replacement
test-upgrade-node-replacement:
	@go test -timeout=60m -v -tags=upgradereplacement -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}

.PHONY: test-create-mixedos
test-create-mixedos:
	@go test -timeout=45m -v -count=1 ./entrypoint/mixedoscluster/... $(if ${SONOBUOY_VERSION},-sonobuoyVersion ${SONOBUOY_VERSION})

.PHONY: test-create-dualstack
test-create-dualstack:
	@go test -timeout=45m -v -count=1 ./entrypoint/dualstack/...

.PHONY: test-version-bump
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


.PHONY: test-etcd-bump
test-etcd-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=etcd \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


.PHONY: test-runc-bump
test-runc-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=runc \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


.PHONY: test-cilium-bump
test-cilium-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cilium \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


.PHONY: test-canal-bump
test-canal-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=canal \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


.PHONY: test-coredns-bump
test-coredns-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=coredns \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})


.PHONY: test-cniplugin-bump
test-cniplugin-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cniplugin \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${APPLY_WORKLOAD},-applyWorkload ${APPLY_WORKLOAD}) \
	$(if ${DELETE_WORKLOAD},-deleteWorkload ${DELETE_WORKLOAD})

.PHONY: test-validate-selinux
test-validate-selinux:
	@go test -timeout=45m -v -count=1 ./entrypoint/selinux/... \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL})

.PHONY: test-restart-service
test-restart-service:
	@go test -timeout=45m -v -count=1 ./entrypoint/restartservice/...

#========================= TestCode Static Quality Check =========================#
.PHONY: pre-commit
pre-commit:
	@gofmt -s -w .
	@goimports -w .
	@go vet ./...
	@golangci-lint run --tests ./...