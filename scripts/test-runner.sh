#!/bin/bash

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
    echo "Test Directory is not set"
    exit 1
fi

if [ -z "${TESTTAG}" ]; then IMGNAME=${IMGNAME}; fi;
echo "Running test: ${TESTTAG} from ${TESTDIR} directory"


if [ -n "${TESTDIR}" ]; then
    if [ "${TESTDIR}" = "upgradecluster" ]; then
        if [ "${TESTTAG}" = "upgrademanual" ]; then
            go test -timeout=45m -v -tags=upgrademanual ./entrypoint/upgradecluster/... -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}" -channel "${CHANNEL}"
        elif [ "${TESTTAG}" = "upgradesuc" ]; then
            go test -timeout=45m -v -tags=upgradesuc ./entrypoint/upgradecluster/... -sucUpgradeVersion "${SUCUPGRADEVERSION}"
        fi
    elif [ "${TESTDIR}" = "versionbump" ]; then
        go test -timeout=45m -v -tags=versionbump ./entrypoint/versionbump/... -cmd "${CMD}" \
            -expectedValue "${EXPECTEDVALUE}" \
            -expectedValueUpgrade "${VALUEUPGRADED}" \
            -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}" \
            -channel "${CHANNEL}" \
            -testCase "${TESTCASE}" \
            -deployWorkload "${DEPLOYWORKLOAD}" \
            -workloadName "${WORKLOADNAME}" \
            -description "${DESCRIPTION}"
    elif [ "${TESTDIR}" = "mixedoscluster" ]; then
        go test -timeout=45m -v ./entrypoint/mixedoscluster/... -sonobuoyVersion "${SONOBUOYVERSION}"
    elif [ "${TESTDIR}" = "dualstack" ]; then
        go test -timeout=45m -v ./entrypoint/dualstack/...
    elif [  "${TESTDIR}" = "createcluster" ]; then
        go test -timeout=45m -v ./entrypoint/createcluster/...
    fi
fi


tail -f /dev/null

