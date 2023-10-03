#!/bin/bash  
#set -x
#echo "$@"

output_files() {
    local instanceState="${1}"
    local resource_name="${2}"

    cd /tmp || exit
    if [ "${1}" == "stop" ]; then    
        aws ec2 describe-instances --filters "Name=tag:Name,Values=${resource_name}-server" \
         "Name=instance-state-name,Values=running" \
        --output text --query 'Reservations[*].Instances[*].InstanceId' > /tmp/ids_server_1_2.txt
        aws ec2 describe-instances --filters "Name=tag:Name,Values=${resource_name}-worker" \
         "Name=instance-state-name,Values=running" \
        --output text --query 'Reservations[*].Instances[*].InstanceId' > /tmp/ids_master_worker.txt    
        cat /tmp/ids_server_1_2.txt /tmp/ids_master_worker.txt > /tmp/ids_all.txt
    fi
}

assign_file(){
    local instanceState="${1}"

    if [ "${1}" == "stop" ]; then
        file="ids_all.txt" 
    elif [ "${1}" == "start_s1_s2" ]; then 
        file="ids_server_1_2.txt" 
    elif [ "${1}" == "start_master_worker" ]; then
        file="ids_master_worker.txt" 
    fi
}

stop_start_nodes(){
    local instanceState="${1}" 

    i=1  
    while read -r line; do    
        if [ "${instanceState}" == "stop" ]; then 
            aws ec2 stop-instances --instance-ids "$line" 
        elif  [[ "${instanceState}" == "start_s1_s2" || "${instanceState}" == "start_master_worker" ]]; then
            aws ec2 start-instances --instance-ids "$line"
            sleep 60
        fi
    i=$((i+1))  
    done < "$file" 
    sleep 120
}

main() {
  output_files "$1" "$2"
  assign_file "$1" 
  stop_start_nodes "$1"
}

main "$@"