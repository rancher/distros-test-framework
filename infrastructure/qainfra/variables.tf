variable "public_ssh_key" {
  description = "Path to public SSH key file"
  type        = string
  default     = ""
}

variable "aws_access_key" {
  description = "AWS access key (empty = use ambient AWS env credentials)"
  type        = string
  default     = ""
}

variable "aws_secret_key" {
  description = "AWS secret key (empty = use ambient AWS env credentials)"
  type        = string
  default     = ""
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
    count         = number
    role          = list(string)
    instance_type = optional(string)
    os            = optional(string, "linux")
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

# Airgap-only inputs, forwarded to cluster_nodes only when airgap_setup is true.
variable "bastion_enabled" {
  description = "Create the bastion host that airgap nodes are reached through."
  type        = bool
  default     = false
}

variable "bastion_instance_type" {
  description = "Bastion instance type; null inherits instance_type."
  type        = string
  default     = null
}

variable "run_id" {
  description = "Identifier of this provisioning run, tagged as RunId on created instances."
  type        = string
  default     = ""
}

variable "qa_infra_sha" {
  description = "Pinned rancher/qa-infra-automation commit, echoed into cluster_nodes_json."
  type        = string
  default     = ""
}

variable "arch" {
  description = "Node CPU architecture (amd64|arm64), echoed into cluster_nodes_json."
  type        = string
  default     = ""
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

variable "aws_ami_windows" {
  description = "Windows Server AMI ID"
  type        = string
  default     = null
}

variable "instance_type_windows" {
  description = "Instance type for Windows agents"
  type        = string
  default     = null
}

variable "aws_volume_size_windows" {
  description = "Root volume size for Windows agents"
  type        = number
  default     = null
}

variable "aws_volume_type_windows" {
  description = "Root volume type for Windows agents"
  type        = string
  default     = null
}

variable "aws_windows_ssh_user" {
  description = "SSH user for Windows agents"
  type        = string
  default     = "Administrator"
}

variable "windows_enable_rdp" {
  description = "Open TCP 3389 for interactive debugging on Windows agents"
  type        = bool
  default     = false
}
