#!/bin/bash

ENV_PRODUCT="${ENV_PRODUCT}"
ENV_TFVARS="${ENV_TFVARS}"
TESTDIR="${TESTDIR}"
TESTTAG="${TESTTAG}"
CMD="${CMD}"
EXPECTEDVALUE="${EXPECTEDVALUE}"
VALUEUPGRADED="${VALUEUPGRADED}"
CHANNEL="${CHANNEL}"
INSTALLVERSIONORCOMMIT="${INSTALLVERSIONORCOMMIT}"
SUCUPGRADEVERSION="${SUCUPGRADEVERSION}"
TESTCASE="${TESTCASE}"
WORKLOADNAME="${WORKLOADNAME}"
DESCRIPTION="${DESCRIPTION}"
DEPLOYWORKLOAD="${DEPLOYWORKLOAD}"
SONOBUOYVERSION="${SONOBUOYVERSION}"

if [ -z "${TESTDIR}" ]; then
    printf "\n\nTESTDIR is not set\n\n"
    exit 1
fi

if [ -z "${IMGNAME}" ]; then
   printf "\n\nIMGNAME is not set\n\n"
   exit 1
fi


case "$TESTDIR" in
     upgradecluster|versionbump|mixedoscluster|dualstack|createcluster)
      printf "\n\nRunning tests for %s\n\n" "${TESTDIR} on ${ENV_PRODUCT}"
        ;;
    *)
        printf "\n\nTESTDIR is not a go test package\n\n"
        exit 1
        ;;
esac

if [ -n "${TESTDIR}" ]; then
    if [ "${TESTDIR}" = "upgradecluster" ]; then
        if [ "${TESTTAG}" = "upgrademanual" ]; then
            go test -timeout=45m -v -tags=upgrademanual -count=1 ./entrypoint/upgradecluster/... -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}" -channel "${CHANNEL}"
        elif [ "${TESTTAG}" = "upgradesuc" ]; then
            go test -timeout=45m -v -tags=upgradesuc -count=1 ./entrypoint/upgradecluster/... -sucUpgradeVersion "${SUCUPGRADEVERSION}"
        fi
    elif [ "${TESTDIR}" = "versionbump" ]; then
        go test -timeout=45m -v -tags=versionbump -count=1 ./entrypoint/versionbump/... \
            -cmd "${CMD}" \
            -expectedValue "${EXPECTEDVALUE}" \
            -expectedValueUpgrade "${VALUEUPGRADED}" \
            -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}" \
            -channel "${CHANNEL}" \
            -testCase "${TESTCASE}" \
            -deployWorkload "${DEPLOYWORKLOAD}" \
            -workloadName "${WORKLOADNAME}" \
            -description "${DESCRIPTION}"
    elif [ "${TESTDIR}" = "mixedoscluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/mixedoscluster/... -sonobuoyVersion "${SONOBUOYVERSION}"
    elif [ "${TESTDIR}" = "dualstack" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/dualstack/...
    elif [  "${TESTDIR}" = "createcluster" ]; then
        go test -timeout=45m -v -count=1 ./entrypoint/createcluster/...
    fi
fi


tail -f /dev/null

