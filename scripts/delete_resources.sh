#!/bin/bash

delete_ec2_instances () {
  EC2_INSTANCE_IDS=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=$1*" "Name=instance-state-name,Values=running" \
    --query 'Reservations[].Instances[].InstanceId' --output text)
  if [ -z "${DRY_RUN}" ]; then
    if [ "${EC2_INSTANCE_IDS}" = "" ];then
      echo "No ec2 instances found with prefix: $1. Nothing to delete."
    else
      echo "Terminating ec2 instances for $1 if still up and running"
      echo "INSTANCE IDs: ${EC2_INSTANCE_IDS}"
      for INSTANCE_ID in ${EC2_INSTANCE_IDS}
      do
        echo "Deleting instance id: ${INSTANCE_ID}"
        aws ec2 terminate-instances --instance-ids "${INSTANCE_ID}" > /dev/null 2>&1
      done
    fi
  else
    echo "EC2 instances matching tag name for the prefix provided $1:"
    EC2_TAG_NAMES=$(aws ec2 describe-instances \
    --filters "Name=tag:Name,Values=archk3s*" "Name=instance-state-name,Values=running" \
    --query 'Reservations[].Instances[].Tags' --output text)
    echo "${EC2_TAG_NAMES}"
    echo "Instance ID List: ${EC2_INSTANCE_IDS}"
    TAG_COUNT=$(echo $EC2_TAG_NAMES |xargs -n1 echo | wc -l)
    echo "EC2 Tag Name count: $((TAG_COUNT/2))"
    INSTANCE_COUNT=$(echo $EC2_INSTANCE_IDS | xargs -n1 echo | wc -l)
    echo "EC2 Instance Id count:$INSTANCE_COUNT"
  fi
}

delete_db_resources () {
  #Search for DB instances and delete them
  DB_INSTANCES=$(aws rds describe-db-instances --query "DBInstances[?starts_with(DBInstanceIdentifier,
  '$1')].DBInstanceIdentifier" --output text 2> /dev/null)

  if [ "${DB_INSTANCES}" = "" ];then
    echo "No db instances found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db instances for $1: $DB_INSTANCES"
      for INSTANCE in $DB_INSTANCES; do
        echo "Deleting db instance: $INSTANCE"
        aws rds delete-db-instance --db-instance-identifier "${INSTANCE}" --skip-final-snapshot > /dev/null 2>&1
      done
    else
      echo "DB Instances that will be deleted: $DB_INSTANCES"
      echo "DB Instance Count:$(echo $DB_INSTANCES |xargs -n1 echo | wc -l)"
    fi
  fi

  #Search for DB clusters and delete them
  CLUSTERS=$(aws rds describe-db-clusters --query "DBClusters[?starts_with(DBClusterIdentifier,
  '$1')].DBClusterIdentifier" --output text 2> /dev/null)
  
  if [ "${CLUSTERS}" = "" ];then
    echo "No db clusters found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db clusters for $1: ${CLUSTERS}"
      for CLUSTER in $CLUSTERS; do
        echo "Deleting cluster: $CLUSTER"
        aws rds delete-db-cluster --db-cluster-identifier "$CLUSTER" --skip-final-snapshot > /dev/null 2>&1
        aws rds wait db-cluster-deleted --db-cluster-identifier "$CLUSTER"
      done
    else
      echo "DB Clusters that will be deleted: $CLUSTERS"
      echo "DB Clusters Count:$(echo $CLUSTERS |xargs -n1 echo | wc -l)"
    fi
  fi

  #Search for DB snapshots and delete them
  SNAPSHOTS=$(aws rds describe-db-snapshots --query "DBSnapshots[?starts_with(DBSnapshotIdentifier,
  '$1')].DBSnapshotIdentifier" --output text 2> /dev/null)

  if [ "${SNAPSHOTS}" = "" ];then
    echo "No db snapshots found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting db snapshots for $1: ${SNAPSHOTS}"
      for SNAPSHOT in $SNAPSHOTS; do
        echo "Deleting db snapshot: $SNAPSHOT"
        aws rds delete-db-snapshot --db-snapshot-identifier "$SNAPSHOT" > /dev/null 2>&1
      done
    else
      echo "DB Snapshots that will be deleted: $SNAPSHOTS"
      echo "DB Snapshots Count:$(echo $SNAPSHOTS |xargs -n1 echo | wc -l)"
    fi
  fi
}

delete_lb_resources () {
  #Get the list of load balancer ARNs
  LB_ARN_LIST=$(aws elbv2 describe-load-balancers \
    --query "LoadBalancers[?starts_with(LoadBalancerName, '$1') && Type=='network'].LoadBalancerArn" \
    --output text)

  if [ "${LB_ARN_LIST}" = "" ];then
    echo "No load balancers found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting load balancers for $1: ${LB_ARN_LIST}"
      #Loop through the load balancer ARNs and delete the load balancers
      for LB_ARN in $LB_ARN_LIST; do
        echo "Deleting load balancer $LB_ARN"
        aws elbv2 delete-load-balancer --load-balancer-arn "$LB_ARN"
      done
    else
      echo "Load Balancers that will be deleted: $LB_ARN_LIST"
      echo "Load Balancers Count:$(echo $LB_ARN_LIST |xargs -n1 echo | wc -l)"
    fi
  fi
}

delete_target_groups () {
  #Get the list of target group ARNs
  TG_ARN_LIST=$(aws elbv2 describe-target-groups \
    --query "TargetGroups[?starts_with(TargetGroupName, '$1') && Protocol=='TCP'].TargetGroupArn" \
    --output text)

  if [ "${TG_ARN_LIST}" = "" ];then
    echo "No target groups found with prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      echo "Deleting target groups for $1: ${TG_ARN_LIST}"
      #Loop through the target group ARNs and delete the target groups
      for TG_ARN in $TG_ARN_LIST; do
        echo "Deleting target group $TG_ARN"
        aws elbv2 delete-target-group --target-group-arn "$TG_ARN"
      done
    else
      echo "Target Groups that will be deleted: $TG_ARN_LIST"
      echo "Target Groups Count:$(echo $TG_ARN_LIST |xargs -n1 echo | wc -l)"
    fi
  fi
}

delete_route53 () {
  #Get the ID and recordName with lower case of the hosted zone that contains the Route 53 record sets
  NAME_PREFIX_LOWER=$(echo "$1" | tr '[:upper:]' '[:lower:]')
  R53_ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "$1." \
    --query "HostedZones[0].Id" --output text)
  R53_RECORD=$(aws route53 list-resource-record-sets \
    --hosted-zone-id "${R53_ZONE_ID}" \
    --query "ResourceRecordSets[?starts_with(Name, '${NAME_PREFIX_LOWER}.') && Type == 'CNAME'].Name" \
    --output text)


  #Get ResourceRecord Value
  RECORD_VALUE=$(aws route53 list-resource-record-sets \
    --hosted-zone-id "${R53_ZONE_ID}" \
    --query "ResourceRecordSets[?starts_with(Name, '${NAME_PREFIX_LOWER}')].ResourceRecords[0].Value" --output text)
  echo "Route53 RECORD_VALUE:
  $RECORD_VALUE"
  RECORD_COUNT=$(echo $RECORD_VALUE | xargs -n1 echo | wc -l)
  echo "Total Route53 Record COUNT is: $RECORD_COUNT"
  if [ "${RECORD_COUNT}" == 0 ]; then
    echo "No Route53 records found for prefix $1. Nothing to delete."
  else
    if [ -z "${DRY_RUN}" ]; then
      for i in $(echo $RECORD_VALUE | xargs -n1 echo)
      do 
          NAME="$(echo "$i" | cut -d "-" -f1)-r53.qa.rancher.space."
          VALUE=$i
          echo "
{\"Changes\": [
        {
            \"Action\": \"DELETE\",
            \"ResourceRecordSet\": {
                \"Name\": \"${NAME}\",
                \"Type\": \"CNAME\",
                \"TTL\": 300,
                \"ResourceRecords\": [
                    {
                        \"Value\": \"${VALUE}\"
                    }
                ]
            }
        }
    ]
}" > payload.json
        cat payload.json
        echo "Deleting Route53 record ${NAME} for prefix $1"
        CHANGE_STATUS=$(aws route53 change-resource-record-sets --hosted-zone-id "${R53_ZONE_ID}" --change-batch file://"${PWD}"/payload.json)
        echo "CHANGE STATUS: 
        $CHANGE_STATUS"
        STATUS_ID=$(echo "$CHANGE_STATUS" | grep Id: | cut -d " " -f 4)
        echo "Status ID: $STATUS_ID"
        #Get status from the change
        aws route53 wait resource-record-sets-changed --id "$STATUS_ID"
        echo "Successfully deleted Route53 record ${NAME}: status: ${STATUS_ID}"
      done
      rm -rf "${PWD}"/payload.json
    fi
  fi
}

delete_all_resources () {
  delete_ec2_instances "$1"
  delete_db_resources "$1"
  delete_lb_resources "$1"
  delete_target_groups "$1"
  delete_route53 "$1"
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

      -r: resource prefix names. can be comma separated. 
      -d: Dry run option. This will simply list the names and ids of what could potentially be listed. But will not delete anything.
      -h: help. Prints usage example.
      
      Ex 1: 
      ./delete_resources.sh -r resourceprefix1,resourceprefix2,resourceprefix2
      - This will delete all resources for the names resourceprefix1,resourceprefix2 and resourceprefix2
      Ex 2:
      ./delete_resources.sh
      - This will lookup the resource name from local tfvars file and delete the resources. This is interactive and will ask for confirmation before delete
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
    echo "Working with local .env and .tfvars file to get resource name for deletion"
    # Find the correct config directory path, based on which directory you are running the script from.
    BASE_DIR=$(echo "$PWD" | sed 's/scripts//')
    CONFIG_DIR="${BASE_DIR}/config"
    echo "config directory path: $CONFIG_DIR"
    #Get resource name from tfvarslocal && change name to make more sense in this context
    # Split string based on delimiter =
    PRODUCT_NAME=$(cat "${CONFIG_DIR}"/.env | grep ENV_PRODUCT | grep -v '#' | cut -d= -f2 | tr -d ' "')
    if echo "${PRODUCT_NAME}" | grep -q "ENV_PRODUCT"; then
      # Split string based on delimiter :
      PRODUCT_NAME=$(echo "${PRODUCT_NAME}" | cut -d ":" -f 2)  
    fi
    echo "PRODUCT NAME is: $PRODUCT_NAME"
  
    if echo "$PRODUCT_NAME" | grep -q "k3s" && echo "$PRODUCT_NAME" | grep -q "rke2"; then
      echo "Please have ONLY ONE product name. Either k3s or rke2. Exiting."
      exit 1
    fi
    if [[ -z "$PRODUCT_NAME" || ! "$PRODUCT_NAME" =~ ^(rke2|k3s)$ ]]; then
      echo "Wrong or empty product name found in .env file for: $PRODUCT_NAME"
      exit 1
    fi

    #Validate path to the tfvars file
    if [[ ! -f "${CONFIG_DIR}"/"$PRODUCT_NAME".tfvars ]]; then
      echo "No $PRODUCT_NAME.tfvars file found in config directory"
      exit 1
    fi

    #Get resource name from tfvars file and validate
    RESOURCE_NAME=$(cat "${CONFIG_DIR}"/"$PRODUCT_NAME".tfvars | grep resource_name | grep -v "#" | cut -d= -f2 | tr -d ' "')
    if [[ -z "$RESOURCE_NAME" ]]; then
      echo "No resource name found for: $PRODUCT_NAME.tfvars file"
      exit 1
    fi

    printf "This is going to delete all AWS resources with the prefix %s. Continue (yes/no)? " "$RESOURCE_NAME"
    read -r REPLY
    if [[ "$REPLY" =~ ^[Yy][Ee][Ss]$ ]]; then
      delete_all_resources "${RESOURCE_NAME}"
    else
      echo "Exiting: No resources deleted as per user input. Please delete the resources manually"
      exit 1
    fi
else
    for i in $(echo "${RESOURCES}" | tr "," "\n")
    do
      PREFIX_LENGTH=${#i}
      if [ "$PREFIX_LENGTH" -gt 5 ]; then
        echo "## For prefix name: $i:"
        delete_all_resources "$i"
      else
        echo "Length of prefix name $i lesser than 5. Please provide a tangible prefix length for deletion."
        exit 1
      fi
    done
fi
