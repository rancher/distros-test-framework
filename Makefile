include ./config/.env

TAGNAME := $(if $(TAGNAME),$(TAGNAME),distros)


test-env-up:
	@docker build . -q -f ./scripts/Dockerfile.build -t acceptance-test-${TAGNAME}

test-run:
	@docker run -dt --name acceptance-test-${IMGNAME} \
	  -e AWS_ACCESS_KEY_ID=$${AWS_ACCESS_KEY_ID} \
	  -e AWS_SECRET_ACCESS_KEY=$${AWS_SECRET_ACCESS_KEY} \
	  --env-file ./config/.env \
	  -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
	  -v ./scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh \
	  acceptance-test-${TAGNAME} && \
	  make image-stats IMGNAME=${IMGNAME} && \
	  make test-logs USE=IMGNAME acceptance-test-${IMGNAME}


## Use this to run on the same environement + cluster from the previous last container -${TAGNAME} created
test-run-state:
	DOCKERCOMMIT=$$? \
	CONTAINER_ID=$(shell docker ps -a -q --filter ancestor=acceptance-test-${TAGNAME} | head -n 1); \
    	if [ -z "$${CONTAINER_ID}" ]; then \
    		echo "No matching container found."; \
    		exit 1; \
    	else \
    		docker commit $$CONTAINER_ID teststate:latest; \
    		if [ $$DOCKERCOMMIT -eq 0 ]; then \
    		  docker run -dt --name acceptance-test-${TESTSTATE} --env-file ./config/.env \
    			-e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID} \
    			-e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY} \
    			-v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
    			-v ./scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh \
    			teststate:latest && \
    			 make test-logs USE=TESTSTATE acceptance-test-${TESTSTATE} \
    			echo "Docker run exit code: $$?"; \
    		else \
    			echo "Failed to commit container"; \
    			exit 1; \
    		fi; \
    	fi

## use this to test a new run on a totally new fresh environment after delete also aws resources
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run

test-logs:
	@if [ "${USE}" = "IMGNAME" ]; then \
		docker logs -f acceptance-test-${IMGNAME}; \
	elif [ "${USE}" = "TESTSTATE" ]; then \
		docker logs -f acceptance-test-${TESTSTATE}; \
	fi;

image-stats:
	@./scripts/docker_stats.sh $$IMGNAME 2>> /tmp/image-${IMGNAME}_stats_output.log &

.PHONY: test-env-down
test-env-down:
	@echo "Removing containers"
	@docker ps -a -q --filter="name=acceptance-test*" | xargs -r docker rm -f 2>/tmp/container_${IMGNAME}.log || true
	@echo "Removing acceptance-test images"
	@docker images -q --filter="reference=acceptance-test*" | xargs -r docker rmi -f  2>/tmp/container_${IMGNAME}.log  || true
	@echo "Removing dangling images"
	@docker images -q -f "dangling=true" | xargs -r docker rmi -f  2>/tmp/container_${IMGNAME}.log || true
	@echo "Removing state images"
	@docker images -q --filter="reference=teststate:latest" | xargs -r docker rmi -f  2>/tmp/container_${IMGNAME}.log  || true

.PHONY: test-env-clean
test-env-clean:
	@./scripts/delete_resources.sh


#========================= Run acceptance tests locally =========================#
.PHONY: remove-tf-state
remove-tf-state:
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform
	@rm -rf ./modules/${ENV_PRODUCT}/.terraform.lock.hcl ./modules/${ENV_PRODUCT}/terraform.tfstate ./modules/${ENV_PRODUCT}/terraform.tfstate.backup


.PHONY: test-create
test-create:
	@go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...


.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion ${SUCUPGRADEVERSION}


.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALLVERSIONORCOMMIT} -channel ${CHANNEL}


.PHONY: test-create-mixedos
test-create-mixedos:
	@go test -timeout=45m -v -count=1 ./entrypoint/mixedoscluster/... $(if ${SONOBUOYVERSION},-sonobuoyVersion ${SONOBUOYVERSION})

.PHONY: test-create-dualstack
test-create-dualstack:
	@go test -timeout=45m -v -count=1 ./entrypoint/dualstack/...

.PHONY: test-version-bump
test-version-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=versionbump \
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
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=etcd \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-runc-bump
test-runc-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=runc \
	-expectedValue ${VALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-cilium-bump
test-cilium-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cilium \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-canal-bump
test-canal-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=canal \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-coredns-bump
test-coredns-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=coredns \
	-expectedValue ${EXPECTEDVALUE} \
	$(if ${VALUEUPGRADED},-expectedValueUpgrade ${VALUEUPGRADED}) \
	$(if ${INSTALLVERSIONORCOMMIT},-installVersionOrCommit ${INSTALLVERSIONORCOMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TESTCASE},-testCase "${TESTCASE}") \
	$(if ${WORKLOADNAME},-workloadName ${WORKLOADNAME}) \
	$(if ${DEPLOYWORKLOAD},-deployWorkload ${DEPLOYWORKLOAD})


.PHONY: test-cniplugin-bump
test-cniplugin-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cniplugin \
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