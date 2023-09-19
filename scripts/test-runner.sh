#!/bin/bash

TESTDIR="${TESTDIR}"
TESTTAG="${TESTTAG}"
CMD="${CMD}"
EXPECTEDVALUE="${EXPECTEDVALUE}"
VALUEUPGRADED="${VALUEUPGRADED}"
CHANNEL="${CHANNEL}"
INSTALLVERSIONORCOMMIT="${INSTALLVERSIONORCOMMIT}"
UPGRADEVERSION="${UPGRADEVERSION}"
TESTCASE="${TESTCASE}"
WORKLOADNAME="${WORKLOADNAME}"
DESCRIPTION="${DESCRIPTION}"
DEPLOYWORKLOAD="${DEPLOYWORKLOAD}"

if [ -z "${TESTDIR}" ]; then
    echo "Test Directory is not set"
    exit 1
fi

cd ./entrypoint || exit
if [ -n "${TESTDIR}" ]; then
    if [ "${TESTDIR}" = "upgradecluster" ]; then
        if [ "${TESTTAG}" = "upgrademanual" ]; then
            go test -timeout=45m -v ./upgradecluster/... -tags=upgrademanual -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}"
        else
            go test -timeout=45m -v -tags=upgradesuc ./upgradecluster/... -upgradeVersion "${UPGRADEVERSION}"
        fi
    elif [ "${TESTDIR}" = "versionbump" ]; then
        go test -timeout=45m -v -tags=versionbump ./versionbump/... -cmd "${CMD}" \
            -expectedValue "${EXPECTEDVALUE}" \
            -expectedValueUpgrade "${VALUEUPGRADED}" \
            -installVersionOrCommit "${INSTALLVERSIONORCOMMIT}" \
            -channel "${CHANNEL}" \
            -testCase "${TESTCASE}" \
            -deployWorkload "${DEPLOYWORKLOAD}" \
            -workloadName "${WORKLOADNAME}" \
            -description "${DESCRIPTION}"
    elif [ "${TESTDIR}" = "mixedoscluster" ]; then
        go test -timeout=45m -v -tags=mixedos ./mixedoscluster/...
    fi
elif [ -z "${TESTDIR}" ]; then
    go test -timeout=45m -v ./createcluster/...
fi

tail -f /dev/null

