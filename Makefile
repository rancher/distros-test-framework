include ./config.mk

TAGNAME ?= default

.PHONY: test-env-up
test-env-up:
	docker build . -q -f ./scripts/Dockerfile.build -t acceptance-test-${TAGNAME}

.PHONY: test-run
test-run:
	@docker run -d --name acceptance-test-${IMGNAME} -t \
      -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
      -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
      -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
      acceptance-test-${TAGNAME} sh -c 'cd ./entrypoint; \
    if [ -n "${TESTDIR}" ]; then \
        if [ "${TESTDIR}" = "upgradecluster" ]; then \
            go test -timeout=45m -v ./upgradecluster/... -installVersionOrCommit "${INSTALLTYPE}"; \
        elif [ "${TESTDIR}" = "versionbump" ]; then \
            go test -timeout=45m -v -tags=versionbump ./versionbump/... -cmd "${CMD}" -expectedValue "${VALUE}" \
            -expectedValueUpgrade "${VALUEUPGRADED}" -installVersionOrCommit "${INSTALLTYPE}" -channel "${CHANNEL}" -testCase "${TESTCASE}" \
            -deployWorkload "${DEPLOYWORKLOAD}" -workloadName "${WORKLOADNAME}" -description "${DESCRIPTION}"; \
		elif [ "${TESTDIR}" = "mixedoscluster" ]; then \
            go test -timeout=45m -v -tags=mixedos ./mixedoscluster/...; \
        fi; \
    elif [ -z "${TESTDIR}" ]; then \
        go test -timeout=45m -v ./createcluster/...; \
    fi;'

.PHONY: test-logs
test-logs:
	@docker logs -f acceptance-test${IMGNAME}

.PHONY: test-env-down
test-env-down:
	@echo "Removing containers and images"
	@docker stop $$(docker ps -a -q --filter="name=acceptance-test*")
	@docker rm $$(docker ps -a -q --filter="name=acceptance-test*") ; \
	 docker rmi --force $$(docker images -q --filter="reference=acceptance-test*")

.PHONY: test-env-clean
test-env-clean:
	@./scripts/delete_resources.sh

.PHONY: test-complete
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run

.PHONY: remove-tf-state
remove-tf-state:
	@rm -rf ./modules/${PRODUCT}/.terraform
	@rm -rf ./modules/${PRODUCT}/.terraform.lock.hcl ./modules/${PRODUCT}/terraform.tfstate ./modules/${PRODUCT}/terraform.tfstate.backup


#========================= Run acceptance tests locally =========================#

.PHONY: test-create
test-create:
	@go test -timeout=45m -v ./entrypoint/createcluster/...

.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc  ./entrypoint/upgradecluster/... -upgradeVersion ${UPGRADEVERSION}

.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALLTYPE}

.PHONY: test-version-bump
test-version-bump:
	  -cmd "${CMD}" \
	  -expectedValue ${VALUE} \
	  -expectedValueUpgrade ${VALUEUPGRADED} \
	  -installVersionOrCommit ${INSTALLTYPE} -channel ${CHANNEL} \
	  -testCase "${TESTCASE}" -deployWorkload ${DEPLOYWORKLOAD} -workloadName ${WORKLOADNAME} -description "${DESCRIPTION}"

.PHONY: test-create-mixedos
test-create-mixedos:
	@go test -timeout=45m -v ./entrypoint/mixedoscluster/...



#========================= TestCode Static Quality Check =========================#
.PHONY: vet-lint                   ## Run locally only inside acceptance framework
vet-lint:
	@echo "Running go vet and lint"
	@go vet ./${TESTDIR} && golangci-lint run --tests



#
#
###########################################################################################
#
#
#include ./config.mk
#
#TAGNAME ?= default
#test-env-up:
#	@cd ../.. && docker build . -q -f ./tests/acceptance/scripts/Dockerfile.build -t rke2-automated-${TAGNAME}
#
#.PHONY: test-run
#test-run:
#	@docker run  --name rke2-automated-test-${IMGNAME} -t \
#      -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
#      -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
#      -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/rke2/tests/acceptance/modules/config/.ssh/aws_key.pem \
#       rke2-automated-${TAGNAME} sh -c 'cd ./tests/acceptance/entrypoint; \
#                                if [ -n "${TESTDIR}" ]; then \
#                                    if [ "${TESTDIR}" = "upgradecluster" ]; then \
#                                        if [ "${TESTTAG}" = "upgradesuc" ]; then \
#                                            go test -timeout=45m -v -tags=upgradesuc ./upgradecluster/... -upgradeVersion "${UPGRADEVERSION}"; \
#                                        elif [ "${TESTTAG}" = "upgrademanual" ]; then \
#                                            go test -timeout=45m -v -tags=upgrademanual ./upgradecluster/... -installVersionOrCommit "${INSTALLTYPE}"; \
#                                        fi; \
#                                    elif [ "${TESTDIR}" = "versionbump" ]; then \
#                                                go test -timeout=45m -v -tags=versionbump ./versionbump/... -cmd "${CMD}" -expectedValue "${VALUE}" \
#                                                -expectedValueUpgrade "${VALUEUPGRADE}" -installVersionOrCommit "${INSTALLTYPE}" -channel "${CHANNEL}" -testCase "${TESTCASE}" \
#                                                -deployWorkload "${DEPLOYWORKLOAD}" -workloadName "${WORKLOADNAME}" -description "${DESCRIPTION}"; \
#                                    fi; \
#                                elif [ -z "${TESTDIR}" ]; then \
#                                    go test -timeout=45m -v ./createcluster/...; \
#                                fi;'
#
#
#.PHONY: test-logs
#test-logs:
#	@docker logs -f rke2-automated-test-${IMGNAME}
#
#
#.PHONY: test-env-down
#test-env-down:
#	@echo "Removing containers and images"
#	@docker stop $$(docker ps -a -q --filter="name=rke2-automated*")
#	@docker rm $$(docker ps -a -q --filter="name=rke2-automated*")
#	@docker rmi $$(docker images -q --filter="reference=rke2-automated*")
#
#
#.PHONY: test-env-clean
#test-env-clean:
#	@./scripts/delete_resources.sh
#
#
#.PHONY: test-complete
#test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run
#
#
#.PHONY: remove-tf-state
#remove-tf-state:
#	@rm -rf ./modules/.terraform
#	@rm -rf ./modules/.terraform.lock.hcl ./modules/terraform.tfstate ./modules/terraform.tfstate.backup
#
#
##======================= Run acceptance tests locally =========================#
#
#.PHONY: test-create
#test-create:
#	@go test -timeout=45m -v ./entrypoint/createcluster/...
#
#
#.PHONY: test-upgrade-suc
#test-upgrade-suc:
#	@go test -timeout=45m -v -tags=upgradesuc  ./entrypoint/upgradecluster/... -upgradeVersion ${UPGRADEVERSION}
#
#
#.PHONY: test-upgrade-manual
#test-upgrade-manual:
#	@go test -timeout=45m -v -tags=upgrademanual ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALLTYPE}
#
#
#.PHONY: test-version-bump
#test-version-bump:
#	go test -timeout=45m -v -tags=versionbump ./entrypoint/versionbump/... \
#	  -cmd "${CMD}" \
#	  -expectedValue ${VALUE} \
#	  -expectedValueUpgrade ${VALUEUPGRADED} \
#	  -installVersionOrCommit ${INSTALLTYPE} -channel ${CHANNEL} \
#	  -testCase "${TESTCASE}" -deployWorkload ${DEPLOYWORKLOAD} -workloadName ${WORKLOADNAME} -description "${DESCRIPTION}"
#
#
##========================= TestCode Static Quality Check =========================#
#.PHONY: vet-lint                      ## Run locally only inside Tests package
#vet-lint:
#	@echo "Running go vet and lint"
#	@go vet ./${TESTDIR} && golangci-lint run --tests