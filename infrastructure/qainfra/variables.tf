variable "user_id" {
  description = "User identifier (usually GitHub username)"
  type        = string
}

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
