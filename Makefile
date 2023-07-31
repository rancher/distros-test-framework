include ./config.mk

TAGNAME ?= default
test-env-up:
	docker build . -q -f ./distros/${DISTRO}/scripts/Dockerfile.build -t ${DISTRO}-automated-${TAGNAME}

.PHONY: test-run
test-run:
	@docker run -d --name ${DISTRO}-automated-test-${IMGNAME} -t \
      -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
      -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
      -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/distros/config/.ssh/aws_key.pem \
       ${DISTRO}-automated-${TAGNAME} sh -c 'cd ./distros/${DISTRO}/feature; \
                                if [ -n "${TESTDIR}" ]; then \
                                    if [ "${TESTDIR}" = "upgradecluster" ]; then \
                                        if [ "${TESTTAG}" = "upgradesuc" ]; then \
                                            go test -timeout=45m -v -tags=upgradesuc ./upgradecluster/... -upgradeVersion "${UPGRADEVERSION}"; \
                                        elif [ "${TESTTAG}" = "upgrademanual" ]; then \
                                            go test -timeout=45m -v -tags=upgrademanual ./upgradecluster/... -installVersionOrCommit "${INSTALLTYPE}"; \
                                        fi; \
                                    elif [ "${TESTDIR}" = "versionbump" ]; then \
                                                go test -timeout=45m -v -tags=versionbump ./versionbump/... -cmd "${CMD}" -expectedValue "${VALUE}" \
                                                -expectedValueUpgrade "${VALUEUPGRADE}" -installVersionOrCommit "${INSTALLTYPE}" -channel "${CHANNEL}" -testCase "${TESTCASE}" \
                                                -deployWorkload "${DEPLOYWORKLOAD}" -workloadName "${WORKLOADNAME}" -description "${DESCRIPTION}"; \
                                    fi; \
                                elif [ -z "${TESTDIR}" ]; then \
                                    go test -timeout=45m -v ./createcluster/... -product "${DISTRO}"; \
                                fi;'


.PHONY: test-logs
test-logs:
	@docker logs -f ${DISTRO}-automated-test-${IMGNAME}


.PHONY: test-env-down
test-env-down:
	@echo "Removing containers and images"
	@docker stop $$(docker ps -a -q --filter="name=${DISTRO}-automated*")
	@docker rm $$(docker ps -a -q --filter="name=${DISTRO}-automated*")
	@docker rmi $$(docker images -q --filter="reference=${DISTRO}-automated*")


.PHONY: test-env-clean
test-env-clean:
	@./distros/${DISTRO}/scripts/delete_resources.sh


.PHONY: test-complete
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run


.PHONY: remove-tf-state
remove-tf-state:
	@rm -rf ./distros/${DISTRO}/modules/.terraform
	@rm -rf ./distros/${DISTRO}/modules/.terraform.lock.hcl ./distros/${DISTRO}/modules/terraform.tfstate ./distros/${DISTRO}/modules/terraform.tfstate.backup


#======================= Run acceptance tests locally =========================#

.PHONY: test-create
test-create:
	@go test -timeout=45m -v ./distros/${DISTRO}/feature/createcluster/... -product "${DISTRO}


.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc  ./distros/${DISTRO}/feature/upgradecluster/... -upgradeVersion ${UPGRADEVERSION} -product "${DISTRO}


.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual ./distros/${DISTRO}/feature/upgradecluster/... -installVersionOrCommit ${INSTALLTYPE}


.PHONY: test-version-bump
test-version-bump:
	go test -timeout=45m -v -tags=versionbump ./distros/${DISTRO}/feature/versionbump/... \
	  -cmd "${CMD}" \
	  -expectedValue ${VALUE} \
	  -expectedValueUpgrade ${VALUEUPGRADED} \
	  -installVersionOrCommit ${INSTALLTYPE} -channel ${CHANNEL} \
	  -testCase "${TESTCASE}" -deployWorkload ${DEPLOYWORKLOAD} -workloadName ${WORKLOADNAME} -description "${DESCRIPTION}"


#========================= TestCode Static Quality Check =========================#
.PHONY: vet-lint                      ## Run locally only inside Tests package
vet-lint:
	@echo "Running go vet and lint"
	@go vet ./${TESTDIR} && golangci-lint run --tests