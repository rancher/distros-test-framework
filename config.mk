SHELL := /bin/bash

LOCAL_TFVARS := $(ENV_TFVARS)

ifeq ($(wildcard ${LOCAL_TFVARS=}),)
  RESOURCE_NAME :=
else
  export RESOURCE_NAME := $(shell sed -n 's/resource_name *= *"\([^"]*\)"/\1/p' ${LOCAL_TFVARS=})
endif

export ACCESS_KEY_LOCAL
export AWS_ACCESS_KEY_ID
export AWS_SECRET_ACCESS_KEY