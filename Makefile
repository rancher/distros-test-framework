include ./config/.env

TAGNAME := $(if $(TAGNAME),$(TAGNAME),default)

.PHONY: test-env-up
test-env-up:
	@docker build . -q -f ./scripts/Dockerfile.build -t acceptance-test-${TAGNAME}

#.PHONY: test-run
#test-run:
#	@docker run -d --name acceptance-test-${IMGNAME} -t \
#      -e AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
#      -e AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
#      -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
#      acceptance-test-${TAGNAME} sh -c 'cd ./entrypoint; \
#if [ -n "${TESTDIR}" ]; then \
#    if [ "${TESTDIR}" = "upgradecluster" ]; then \
#        if [ "${TESTTAG}" = "upgrademanual" ]; then \
#            go test -timeout=45m -v ./upgradecluster/... -tags=upgrademanual -installVersionOrCommit "${INSTALLTYPE}"; \
#        else \
#            go test -timeout=45m -v ./upgradecluster/... -tags=upgradesuc -upgradeVersion "${UPGRADEVERSION}"; \
#        fi; \
#    elif [ "${TESTDIR}" = "versionbump" ]; then \
#        go test -timeout=45m -v -tags=versionbump ./versionbump/... \
#            -cmd "${CMD}" \
#            -expectedValue "${EXPECTEDVALUE}" \
#            -expectedValueUpgrade "${VALUEUPGRADED}" \
#            -installVersionOrCommit "${INSTALLTYPE}" \
#            -channel "${CHANNEL}" \
#            -testCase "${TESTCASE}" \
#            -deployWorkload "${DEPLOYWORKLOAD}" \
#            -workloadName "${WORKLOADNAME}" \
#            -description "${DESCRIPTION}"; \
#    elif [ "${TESTDIR}" = "mixedoscluster" ]; then \
#        go test -timeout=45m -v -tags=mixedos ./mixedoscluster/...; \
#    fi; \
#elif [ -z "${TESTDIR}" ]; then \
#    go test -timeout=45m -v ./createcluster/...; \
#fi; \
#'

test-run:
	if [ -z "$${IMGNAME}" ]; then IMGNAME=${IMGNAME}; fi; \
	if [ -z "$${ACCESS_KEY_LOCAL}" ]; then ACCESS_KEY_LOCAL=${ACCESS_KEY_LOCAL}; fi; \
	if [ -z "$${TAGNAME}" ]; then TAGNAME=${TAGNAME}; fi; \
	@docker run -d --name acceptance-test-$${IMGNAME} -t \
	  -e AWS_ACCESS_KEY_ID=$${AWS_ACCESS_KEY_ID} \
	  -e AWS_SECRET_ACCESS_KEY=$${AWS_SECRET_ACCESS_KEY} \
	  --env-file ./config/.env \
	  -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
	  -v ./scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh \
	  acceptance-test-$${TAGNAME} sh ./scripts/test-runner.sh


test-run-state:
	@CONTAINER_ID=$(docker ps -a -q --filter ancestor=acceptance-test-${TAGNAME} | head -n 1); \
    	if [ -z "$$CONTAINER_ID" ]; then \
    		echo "No matching container found."; \
    		exit 1; \
    	else \
    		echo "Committing container $$CONTAINER_ID"; \
    		@docker commit $$CONTAINER_ID teststate:latest; \
    		if [ $$? -eq 0 ]; then \
    			docker run -d --name "${TESTSTATE}" --env-file ./config/.env  -t teststate:latest \
    			-e AWS_ACCESS_KEY_ID=$${AWS_ACCESS_KEY_ID} \
    			-e AWS_SECRET_ACCESS_KEY=$${AWS_SECRET_ACCESS_KEY} \
    			-v $${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
    			-v ./scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh \
    			sh ./scripts/test-runner.sh \
    		else \
    			echo "Failed to commit container"; \
    			exit 1; \
    		fi; \
    	fi

## USE THIS TO RUN TESTS IN A TOTALLY FRESH ENVIRONMENT IN DOCKER
.PHONY: test-complete
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run


.PHONY: test-logs
test-logs:
	@docker logs -f acceptance-test${IMGNAME}

.PHONY: test-env-down
test-env-down:
	@echo "Removing containers and images"
	@docker rm --force $$(docker ps -a -q --filter="name=acceptance-test*") ; \
	 docker rmi --force $$(docker images -q --filter="reference=acceptance-test*") ; \
	 docker rmi --force $$(docker images -q --filter="reference=teststate*")

.PHONY: test-env-clean
test-env-clean:
	@yes | ./scripts/delete_resources.sh


#========================= Run acceptance tests locally =========================#
.PHONY: remove-tf-state
remove-tf-state:
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform.lock.hcl ./modules/${ENV_PRODUCT}/terraform.tfstate ./modules/${ENV_PRODUCT}/terraform.tfstate.backup


.PHONY: test-create
test-create:
	@go test -timeout=45m -v ./entrypoint/createcluster/...


.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc  ./entrypoint/upgradecluster/... -sucUpgradeVersion ${SUCUPGRADEVERSION}


.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALLVERSIONORCOMMIT}


.PHONY: test-create-mixedos
test-create-mixedos:
	@go test -timeout=45m -v ./entrypoint/mixedoscluster/...


.PHONY: test-version-bump
test-version-bump:
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=versionbump \
	-cmd "${CMD}" \
    -expectedValue ${EXPECTEDVALUE} \
    $(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DESCRIPTION},-description "${DESCRIPTION}") \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD}) \


.PHONY: test-etcd-bump
test-etcd-bump:
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=etcd \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-runc-bump
test-runc-bump:
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=runc \
	-expectedValue ${VALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-cilium-bump
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=cilium \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-canal-bump
test-canal-bump:
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=canal \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-coredns-bump
test-coredns-bump:
	@go test -timeout=45m -v ./entrypoint/versionbump/... -tags=coredns \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-cniplugin-bump
test-cniplugin-bump:
	@go test -timeout=45m -v ./entrypoint/cnipluginversionbump/... -tags=cniplugin \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})

#========================= TestCode Static Quality Check =========================#
.PHONY: vet-lint
vet-lint:
	@echo "Running go vet and lint"
	@go vet ./${TESTDIR} && golangci-lint run --tests