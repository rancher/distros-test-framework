include ./config/.env

TAG_NAME := $(if $(TAG_NAME),$(TAG_NAME),distros)

test-env-up:
	@docker build . -q -f ./scripts/Dockerfile.build -t acceptance-test-${TAG_NAME}

test-run:
	@docker run -dt --name acceptance-test-${IMG_NAME} \
	  -e AWS_ACCESS_KEY_ID=$${AWS_ACCESS_KEY_ID} \
	  -e AWS_SECRET_ACCESS_KEY=$${AWS_SECRET_ACCESS_KEY} \
	  -e TEST_DIR=${TEST_DIR} \
	  -e IMG_NAME=${IMG_NAME} \
	  -e TEST_TAG=${TEST_TAG} \
	  --env-file ./config/.env \
	  -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
	  acceptance-test-${TAG_NAME} && \
	  make image-stats IMG_NAME=${IMG_NAME} && \
	  make test-logs USE=IMG_NAME acceptance-test-${IMG_NAME}

## Use this to run automatically without need to change image name
test-run-new:
	$(eval RANDOM_SUFFIX := $(shell LC_ALL=C < /dev/urandom tr -dc 'a-z' | head -c3))
	@NEW_IMG_NAME="" ; \
	if [[ ! -z "${RKE2_VERSION}" ]]; then \
		NEW_IMG_NAME=$$(echo ${RKE2_VERSION} | sed 's/+.*//'); \
	elif [[ ! -z "${K3S_VERSION}" ]]; then \
		NEW_IMG_NAME=$$(echo ${K3S_VERSION} | sed 's/+.*//'); \
	fi; \
	docker run -dt --name acceptance-test-${IMG_NAME}-$${NEW_IMG_NAME}-${RANDOM_SUFFIX} \
	  -e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID} \
	  -e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY} \
	  --env-file ./config/.env \
	  -v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
	  acceptance-test-${TAG_NAME} && \
	  make image-stats IMG_NAME=${IMG_NAME}-$${NEW_IMG_NAME}-${RANDOM_SUFFIX} && \
	  docker logs -f acceptance-test-${IMG_NAME}-$${NEW_IMG_NAME}-${RANDOM_SUFFIX}

## Use this to build and run automatically
test-build-run:
	@make test-env-up && \
	make test-run-new

## Use this to run on the same environement + cluster from the previous last container -${TAGNAME} created
test-run-state:
	DOCKER_COMMIT=$$? \
	CONTAINER_ID=$(shell docker ps -a -q --filter ancestor=acceptance-test-${TAG_NAME} | head -n 1); \
    	if [ -z "$${CONTAINER_ID}" ]; then \
    		echo "No matching container found."; \
    		exit 1; \
    	else \
    		docker commit $$CONTAINER_ID teststate:latest; \
    		if [ $$DOCKER_COMMIT -eq 0 ]; then \
    		  docker run -dt --name acceptance-test-${TEST_STATE} --env-file ./config/.env \
    			-e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID} \
    			-e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY} \
    			-v ${ACCESS_KEY_LOCAL}:/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem \
    			-v ./scripts/test-runner.sh:/go/src/github.com/rancher/distros-test-framework/scripts/test-runner.sh \
    			teststate:latest && \
    			 make test-logs USE=TEST_STATE acceptance-test-${TEST_STATE} \
    			echo "Docker run exit code: ${$?}"; \
    		else \
    			echo "Failed to commit container"; \
    			exit 1; \
    		fi; \
    	fi

## use this to test a new run on a totally new fresh environment after delete also aws resources
test-complete: test-env-clean test-env-down remove-tf-state test-env-up test-run

## use this to skip tests
test-skip:
	ifdef SKIP
		SKIP_FLAG=--ginkgo.skip="${SKIP}"
	endif

test-logs:
	@if [ "${USE}" = "IMG_NAME" ]; then \
		docker logs -f acceptance-test-${IMG_NAME}; \
	elif [ "${USE}" = "TEST_STATE" ]; then \
		docker logs -f acceptance-test-${TEST_STATE}; \
	fi;

image-stats:
	@./scripts/docker_stats.sh $$IMG_NAME 2>> /tmp/image-${IMG_NAME}_stats_output.log &

.PHONY: test-env-down
test-env-down:
	@echo "Removing containers"
	@docker ps -a -q --filter="name=acceptance-test*" | xargs -r docker rm -f 2>/tmp/container_${IMG_NAME}.log || true
	@echo "Removing acceptance-test images"
	@docker images -q --filter="reference=acceptance-test*" | xargs -r docker rmi -f  2>/tmp/container_${IMG_NAME}.log  || true
	@echo "Removing dangling images"
	@docker images -q -f "dangling=true" | xargs -r docker rmi -f  2>/tmp/container_${IMG_NAME}.log || true
	@echo "Removing state images"
	@docker images -q --filter="reference=teststate:latest" | xargs -r docker rmi -f  2>/tmp/container_${IMG_NAME}.log  || true

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


.PHONY: test-validate
test-validate:
	@go test -timeout=45m -v -count=1 ./entrypoint/validatecluster/...


.PHONY: test-upgrade-suc
test-upgrade-suc:
	@go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion ${SUC_UPGRADE_VERSION}


.PHONY: test-upgrade-manual
test-upgrade-manual:
	@go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT} -channel ${CHANNEL}


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
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD}) \


.PHONY: test-etcd-bump
test-etcd-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=etcd \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})


.PHONY: test-runc-bump
test-runc-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=runc \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})


.PHONY: test-cilium-bump
test-cilium-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cilium \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})


.PHONY: test-canal-bump
test-canal-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=canal \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})


.PHONY: test-coredns-bump
test-coredns-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=coredns \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})


.PHONY: test-cniplugin-bump
test-cniplugin-bump:
	@go test -timeout=45m -v -count=1 ./entrypoint/versionbump/... -tags=cniplugin \
	-expectedValue ${EXPECTED_VALUE} \
	$(if ${VALUE_UPGRADED},-expectedValueUpgrade ${VALUE_UPGRADED}) \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL}) \
	$(if ${TEST_CASE},-testCase "${TEST_CASE}") \
	$(if ${WORKLOAD_NAME},-workloadName ${WORKLOAD_NAME}) \
	$(if ${DEPLOY_WORKLOAD},-deployWorkload ${DEPLOY_WORKLOAD})

.PHONY: test-validate-selinux
test-validate-selinux:
	@go test -timeout=45m -v -count=1 ./entrypoint/selinux/... \
	$(if ${INSTALL_VERSION_OR_COMMIT},-installVersionOrCommit ${INSTALL_VERSION_OR_COMMIT}) \
	$(if ${CHANNEL},-channel ${CHANNEL})

#========================= TestCode Static Quality Check =========================#
.PHONY: vet-lint
vet-lint:
	@echo "Running go vet and lint"
	@go vet ./${TEST_DIR} && golangci-lint run --tests