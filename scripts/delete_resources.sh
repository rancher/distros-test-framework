#!/bin/bash

# Match mode. The env-derived path matches the specific "dsf-<RESOURCE_NAME>-"
# token and needs substring matching because EC2 tags are "tf-dsf-<name>-...",
# so it exports MATCH_CONTAINS=1. The -r manual path (Jenkins calls it directly,
# no confirmation) keeps prefix matching (starts_with) so an arbitrary token
# can't destructively over-match unrelated resources.
match_op () { [ -n "${MATCH_CONTAINS}" ] && echo "contains" || echo "starts_with"; }
ec2_glob () { [ -n "${MATCH_CONTAINS}" ] && echo "*$1*" || echo "$1*"; }

delete_ec2_instances () {
  local GLOB rc=0
  GLOB=$(ec2_glob "$1")
  EC2_INSTANCE_IDS=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=${GLOB}" "Name=instance-state-name,Values=running" \
    --query 'Reservations[].Instances[].InstanceId' --output text) \
    || { echo "Failed to describe ec2 instances for $1"; return 1; }
  if [ -z "${DRY_RUN}" ]; then
    if [ "${EC2_INSTANCE_IDS}" = "" ];then
      echo "No ec2 instances found with prefix: $1 Nothing to delete."
    else
      echo "Terminating ec2 instances for $1 if still up and running"
      echo "INSTANCE IDs: ${EC2_INSTANCE_IDS}"
      for INSTANCE_ID in ${EC2_INSTANCE_IDS}
      do
        echo "Deleting instance id: ${INSTANCE_ID}"
        aws ec2 terminate-instances --instance-ids "${INSTANCE_ID}" > /dev/null 2>&1 \
          || { echo "Failed to terminate instance ${INSTANCE_ID}"; rc=1; }
      done
    fi
  else
    echo "EC2 instances matching tag name for the prefix provided $1:"
    EC2_TAG_NAMES=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=${GLOB}" "Name=instance-state-name,Values=running" \
    --query 'Reservations[].Instances[].Tags' --output text)
    echo "${EC2_TAG_NAMES}"
    echo "Instance ID List: ${EC2_INSTANCE_IDS}"
    TAG_COUNT=$(echo $EC2_TAG_NAMES |xargs -n1 echo | wc -l)
    echo "EC2 Tag Name count: $((TAG_COUNT/2))"
    INSTANCE_COUNT=$(echo $EC2_INSTANCE_IDS | xargs -n1 echo | wc -l)
    echo "EC2 Instance Id count:$INSTANCE_COUNT"
  fi

  return "${rc}"
}

delete_db_resources () {
  local rc=0
  #Search for DB instances and delete them
  DB_INSTANCES=$(aws rds describe-db-instances --query "DBInstances[?$(match_op)(DBInstanceIdentifier, '$1')].DBInstanceIdentifier" --output text 2> /dev/null)

  if [ "${DB_INSTANCES}" = "" ];then
    echo "No db instances found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db instances for $1: $DB_INSTANCES"
      for INSTANCE in $DB_INSTANCES; do
        echo "Deleting db instance: $INSTANCE"
        aws rds delete-db-instance --db-instance-identifier "${INSTANCE}" --skip-final-snapshot > /dev/null 2>&1 \
          || { echo "Failed to delete db instance ${INSTANCE}"; rc=1; }
        # Wait for full deletion before continuing cleanup.
        aws rds wait db-instance-deleted --db-instance-identifier "${INSTANCE}" \
          || { echo "Timed out waiting for db instance ${INSTANCE} deletion"; rc=1; }
      done
    else
      echo "DB Instances that will be deleted: $DB_INSTANCES"
      echo "DB Instance Count:$(echo $DB_INSTANCES |xargs -n1 echo | wc -l)"
    fi
  fi

  #Search for DB clusters and delete them
  CLUSTERS=$(aws rds describe-db-clusters --query "DBClusters[?$(match_op)(DBClusterIdentifier, '$1')].DBClusterIdentifier" --output text 2> /dev/null)
  
  if [ "${CLUSTERS}" = "" ];then
    echo "No db clusters found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db clusters for $1: ${CLUSTERS}"
      for CLUSTER in $CLUSTERS; do
        echo "Deleting cluster: $CLUSTER"
        aws rds delete-db-cluster --db-cluster-identifier "$CLUSTER" --skip-final-snapshot > /dev/null 2>&1 \
          || { echo "Failed to delete db cluster ${CLUSTER}"; rc=1; }
        aws rds wait db-cluster-deleted --db-cluster-identifier "$CLUSTER" \
          || { echo "Timed out waiting for db cluster ${CLUSTER} deletion"; rc=1; }
      done
    else
      echo "DB Clusters that will be deleted: $CLUSTERS"
      echo "DB Clusters Count:$(echo $CLUSTERS |xargs -n1 echo | wc -l)"
    fi
  fi

  #Search for DB snapshots and delete them
  SNAPSHOTS=$(aws rds describe-db-snapshots --query "DBSnapshots[?$(match_op)(DBSnapshotIdentifier, '$1')].DBSnapshotIdentifier" --output text 2> /dev/null)

  if [ "${SNAPSHOTS}" = "" ];then
    echo "No db snapshots found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db snapshots for $1: ${SNAPSHOTS}"
      for SNAPSHOT in $SNAPSHOTS; do
        echo "Deleting db snapshot: $SNAPSHOT"
        aws rds delete-db-snapshot --db-snapshot-identifier "$SNAPSHOT" > /dev/null 2>&1 \
          || { echo "Failed to delete db snapshot ${SNAPSHOT}"; rc=1; }
      done
    else
      echo "DB Snapshots that will be deleted: $SNAPSHOTS"
      echo "DB Snapshots Count:$(echo $SNAPSHOTS |xargs -n1 echo | wc -l)"
    fi
  fi

  return "${rc}"
}

delete_lb_resources () {
  local rc=0
  #Get the list of load balancer ARNs
  LB_ARN_LIST=$(aws elbv2 describe-load-balancers \
    --query "LoadBalancers[?$(match_op)(LoadBalancerName, '$1') && Type=='network'].LoadBalancerArn" \
    --output text) || { echo "Failed to describe load balancers for $1"; return 1; }

  if [ "${LB_ARN_LIST}" = "" ];then
    echo "No load balancers found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting load balancers for $1: ${LB_ARN_LIST}"
      #Loop through the load balancer ARNs and delete the load balancers
      for LB_ARN in $LB_ARN_LIST; do
        echo "Deleting load balancer $LB_ARN"
        aws elbv2 delete-load-balancer --load-balancer-arn "$LB_ARN" \
          || { echo "Failed to delete load balancer ${LB_ARN}"; rc=1; }
      done
    else
      echo "Load Balancers that will be deleted: $LB_ARN_LIST"
      echo "Load Balancers Count:$(echo $LB_ARN_LIST |xargs -n1 echo | wc -l)"
    fi
  fi

  return "${rc}"
}

delete_target_groups () {
  local rc=0
  #Get the list of target group ARNs
  TG_ARN_LIST=$(aws elbv2 describe-target-groups \
    --query "TargetGroups[?$(match_op)(TargetGroupName, '$1') && Protocol=='TCP'].TargetGroupArn" \
    --output text) || { echo "Failed to describe target groups for $1"; return 1; }

  if [ "${TG_ARN_LIST}" = "" ];then
    echo "No target groups found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting target groups for $1: ${TG_ARN_LIST}"
      #Loop through the target group ARNs and delete the target groups
      for TG_ARN in $TG_ARN_LIST; do
        echo "Deleting target group $TG_ARN"
        aws elbv2 delete-target-group --target-group-arn "$TG_ARN" \
          || { echo "Failed to delete target group ${TG_ARN}"; rc=1; }
      done
    else
      echo "Target Groups that will be deleted: $TG_ARN_LIST"
      echo "Target Groups Count:$(echo $TG_ARN_LIST |xargs -n1 echo | wc -l)"
    fi
  fi

  return "${rc}"
}

delete_route53 () {
  local OP NAME_LOWER
  OP=$(match_op)
  NAME_LOWER=$(echo "$1" | tr '[:upper:]' '[:lower:]')
  R53_ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "$1." \
    --query "HostedZones[0].Id" --output text)

  # Fetch the matched CNAME record sets as "name<TAB>value" pairs and delete each
  # using its ACTUAL name + value. Do not reconstruct the name from the value.
  RECORDS=$(aws route53 list-resource-record-sets \
    --hosted-zone-id "${R53_ZONE_ID}" \
    --query "ResourceRecordSets[?${OP}(Name, '${NAME_LOWER}') && Type == 'CNAME'].[Name, ResourceRecords[0].Value]" \
    --output text)

  if [ -z "${RECORDS}" ]; then
    echo "No Route53 CNAME records found for $1. Nothing to delete."
    return
  fi

  echo "Route53 CNAME records matching $1:"
  echo "${RECORDS}"

  if [ -n "${DRY_RUN}" ]; then
    echo "Route53 record count: $(echo "${RECORDS}" | grep -c .)"
    return
  fi

  while IFS=$'\t' read -r NAME VALUE; do
    [ -z "${NAME}" ] && continue
    cat > "${PWD}/payload.json" <<EOF
{"Changes": [{"Action": "DELETE", "ResourceRecordSet": {"Name": "${NAME}", "Type": "CNAME", "TTL": 300, "ResourceRecords": [{"Value": "${VALUE}"}]}}]}
EOF
    echo "Deleting Route53 record ${NAME} -> ${VALUE}"
    # --query/--output text makes this independent of the CLI default output
    # format (Jenkins sets output = yaml). Fail hard if the change is rejected.
    STATUS_ID=$(aws route53 change-resource-record-sets \
      --hosted-zone-id "${R53_ZONE_ID}" \
      --change-batch "file://${PWD}/payload.json" \
      --query 'ChangeInfo.Id' --output text) || {
      echo "Failed to delete Route53 record ${NAME}"
      rm -f "${PWD}/payload.json"
      return 1
    }
    aws route53 wait resource-record-sets-changed --id "${STATUS_ID}" || {
      echo "Timed out waiting for Route53 delete ${NAME}"
      rm -f "${PWD}/payload.json"
      return 1
    }
    echo "Deleted ${NAME}"
  done <<< "${RECORDS}"
  rm -f "${PWD}/payload.json"
}

delete_all_resources () {
  # Attempt every resource type, but remember any failure so the caller (and
  # Jenkins) can't pass while a resource lingers.
  local rc=0
  delete_ec2_instances "$1" || rc=1
  delete_db_resources "$1" || rc=1
  delete_lb_resources "$1" || rc=1
  delete_target_groups "$1" || rc=1
  delete_route53 "$1" || rc=1

  return "${rc}"
}


while getopts r:dh OPTION
do 
  case "${OPTION}"
    in
    r) RESOURCES=${OPTARG};;
    d) DRY_RUN="true";;
    h|?)
      echo "
    Usage: 
      $(basename "$0") [-r <resource_prefix>] [-d] [-h]

      -r: resource prefix names (comma separated). Matched as a leading prefix
          (starts_with) — pass a full, specific token to avoid over-matching.
      -d: Dry run option. This will simply list the names and ids of what could potentially be listed. But will not delete anything.
      -h: help. Prints usage example.
      
      Ex 1: 
      ./delete_resources.sh -r resourceprefix1,resourceprefix2,resourceprefix2
      - This will delete all resources for the names resourceprefix1,resourceprefix2 and resourceprefix2
      Ex 2:
      ./delete_resources.sh
      - Reads RESOURCE_NAME (and ENV_PRODUCT for sanity checking) from
        config/.env, matches resources whose name contains "dsf-<RESOURCE_NAME>-"
        (catches tf-prefixed EC2 tags too), and deletes them after confirmation.
      Ex 3: 
      ./delete_resources.sh -h
      - Print usage details for reference.
      Ex 4:
      ./delete_resources.sh -r resourceprefix1 -d
      - This is a dry run. You will see results of what may potentially get deleted but will not do any actual deletion.
      "
      exit 1
      ;;
  esac
done

if [ "${RESOURCES}" = "" ]; then
    echo "Reading config/.env to derive the AWS resource prefix"
    # Find the correct config directory path, based on which directory you are running the script from.
    BASE_DIR=$(echo "$PWD" | sed 's/scripts//')
    CONFIG_DIR="${BASE_DIR}/config"
    ENV_FILE="${CONFIG_DIR}/.env"
    echo "config directory path: $CONFIG_DIR"

    if [[ ! -f "${ENV_FILE}" ]]; then
      echo "No .env file found at ${ENV_FILE}"
      exit 1
    fi

    # Helper: read a KEY=VALUE pair from .env, ignoring comments and stripping
    # surrounding quotes/whitespace. Picks the last non-comment occurrence.
    read_env_var () {
      grep -E "^[[:space:]]*$1=" "${ENV_FILE}" \
        | grep -v '^[[:space:]]*#' \
        | tail -n1 \
        | cut -d= -f2- \
        | tr -d ' "'
    }

    PRODUCT_NAME=$(read_env_var ENV_PRODUCT)
    echo "PRODUCT NAME is: ${PRODUCT_NAME:-<unset>}"

    if [[ -n "${PRODUCT_NAME}" && ! "${PRODUCT_NAME}" =~ ^(rke2|k3s)$ ]]; then
      echo "Unexpected ENV_PRODUCT value in .env: '${PRODUCT_NAME}' (expected rke2 or k3s)"
      exit 1
    fi

    # The framework names resources with the token "dsf-<RESOURCE_NAME>-", but
    # not always at the start: EC2 tags are "tf-dsf-<RESOURCE_NAME>-...", while
    # LB/TG/Route53 are "dsf-<RESOURCE_NAME>-...". So we match that token as a
    # substring (contains), not a leading prefix — this catches the tf-prefixed
    # EC2 instances too, while staying specific enough (dsf-<name>-) that it
    # never matches another user's resources.
    RESOURCE_NAME=$(read_env_var RESOURCE_NAME)
    if [[ -z "${RESOURCE_NAME}" ]]; then
      echo "RESOURCE_NAME is not set in ${ENV_FILE}"
      exit 1
    fi

    PREFIX="dsf-${RESOURCE_NAME}-"
    echo "Matching AWS resources containing: ${PREFIX}"

    printf "This is going to delete all AWS resources whose name contains '%s'\nContinue (yes/no)? " "${PREFIX}"
    read -r REPLY
    if [[ "$REPLY" =~ ^[Yy][Ee][Ss]$ ]]; then
      # Substring match: the token "dsf-<name>-" is specific, and EC2 tags carry
      # a "tf-" prefix so starts_with would miss them.
      MATCH_CONTAINS=1
      delete_all_resources "${PREFIX}" || { echo "Cleanup failed for ${PREFIX}"; exit 1; }
    else
      echo "Exiting: No resources deleted as per user input. Please delete the resources manually"
      exit 1
    fi
else
    FAILED=0
    for i in $(echo "${RESOURCES}" | tr "," "\n")
    do
      PREFIX_LENGTH=${#i}
      if [ "$PREFIX_LENGTH" -gt 5 ]; then
        echo "## For prefix name: $i:"
        # Attempt every prefix, but remember if any failed so the job (Jenkins
        # passes comma-separated prefixes) doesn't pass on a partial cleanup.
        delete_all_resources "$i" || { echo "Cleanup failed for prefix: $i"; FAILED=1; }
      else
        echo "Length of prefix name $i lesser than 5. Please provide a tangible prefix length for deletion."
        exit 1
      fi
    done
    if [ "$FAILED" -ne 0 ]; then
      echo "One or more prefixes failed to clean up."
      exit 1
    fi
fi
