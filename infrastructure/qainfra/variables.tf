variable "public_ssh_key" {
  description = "Path to public SSH key file"
  type        = string
  default     = ""
}

variable "aws_access_key" {
  description = "AWS access key"
  type        = string
}

variable "aws_secret_key" {
  description = "AWS secret key"
  type        = string
}

variable "aws_region" {
  description = "AWS region"
  type        = string
}

variable "aws_ami" {
  description = "AWS AMI ID"
  type        = string
}

variable "aws_hostname_prefix" {
  description = "Hostname prefix for instances"
  type        = string
}

variable "aws_route53_zone" {
  description = "Route53 zone"
  type        = string
}

variable "aws_ssh_user" {
  description = "SSH user for instances"
  type        = string
}

variable "aws_security_group" {
  description = "List of security group IDs"
  type        = list(string)
}

variable "aws_vpc" {
  description = "VPC ID"
  type        = string
}

variable "aws_volume_size" {
  description = "Root volume size"
  type        = number
}

variable "aws_volume_type" {
  description = "Root volume type"
  type        = string
}

variable "aws_subnet" {
  description = "Subnet ID"
  type        = string
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
}

variable "nodes" {
  description = "Configuration for cluster nodes"
  type = list(object({
    count = number
    role  = list(string)
  }))
}

variable "airgap_setup" {
  description = "Whether this is an airgap setup"
  type        = bool
  default     = false
}

variable "proxy_setup" {
  description = "Whether this is a proxy setup"
  type        = bool
  default     = false
}

variable "create_eip" {
  description = "Allocate Elastic IPs and associate them with each node so reboots keep stable public addresses. Required by the rebootinstances test."
  type        = bool
  default     = false
}

variable "datastore_type" {
  description = "etcd (embedded, no DB) or external (provision RDS)."
  type        = string
  default     = "etcd"
}

variable "external_db" {
  description = "RDS engine: postgres | mysql | mariadb | aurora-mysql."
  type        = string
  default     = ""
}

variable "external_db_version" {
  description = "RDS engine version."
  type        = string
  default     = ""
}

variable "external_db_group_name" {
  description = "DB parameter group name."
  type        = string
  default     = ""
}

variable "external_db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t3.medium"
}

variable "external_db_username" {
  type    = string
  default = "adminuser"
}

variable "external_db_password" {
  type      = string
  default   = "admin1234"
  sensitive = true
}

variable "external_db_engine_mode" {
  description = "Aurora engine mode."
  type        = string
  default     = "provisioned"
}

variable "external_db_subnet_ids" {
  description = "Subnet IDs (>=2 AZs) for the RDS subnet group. Empty uses the account default subnet group."
  type        = list(string)
  default     = []
}
