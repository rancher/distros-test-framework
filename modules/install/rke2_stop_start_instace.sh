#!/bin/bash  
#set -x
echo "$@"
type=$1
resource_name=$2

cd /tmp
if [ "$type" == "stop" ]; then    
    #output instances IDs

    aws ec2 describe-instances --filters "Name=tag:Name,Values=${resource_name}-server1,${resource_name}-server2" \
     "Name=instance-state-name,Values=running" \
    --output text --query 'Reservations[*].Instances[*].InstanceId' > /tmp/ids_server_1_2.txt

    aws ec2 describe-instances --filters "Name=tag:Name,Values=${resource_name}-server,${resource_name}-worker" \
     "Name=instance-state-name,Values=running" \
    --output text --query 'Reservations[*].Instances[*].InstanceId' > /tmp/ids_master_worker.txt    

    cat /tmp/ids_server_1_2.txt /tmp/ids_master_worker.txt > /tmp/ids_all.txt
fi

if [ "$type" == "stop" ]; then
    file="ids_all.txt" 
elif [ "$type" == "start_s1_s2" ]; then 
    file="ids_server_1_2.txt" 
elif [ "$type" == "start_master_worker" ]; then
    file="ids_master_worker.txt" 
fi

i=1  
while read line; do    
    if [ "$type" == "stop" ]; then 
        aws ec2 stop-instances --instance-ids $line 
    elif  [[ "$type" == "start_s1_s2" || "$type" == "start_master_worker" ]]; then
        aws ec2 start-instances --instance-ids $line
        sleep 60
    fi
i=$((i+1))  
done < $file 
sleep 120

