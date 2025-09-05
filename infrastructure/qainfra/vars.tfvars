user_id             = "distros-qa-"
aws_hostname_prefix = "distros-qa-"
public_ssh_key      = ""
aws_access_key      = "" 
aws_secret_key      = "" 
aws_region          = "us-east-2"
aws_ami             = "ami-0f6c1051253397fef"
instance_type       = "t3.xlarge"
aws_security_group  = ["sg-08e8243a8cfbea8a0"]
aws_subnet          = "subnet-1ed44d64"
aws_vpc             = "vpc-bfccf4d7"  
aws_route53_zone    = "qa.rancher.space" 
aws_ssh_user        = "ec2-user" 
aws_volume_size     = 40 
aws_volume_type     = "gp3"
airgap_setup        = false
proxy_setup         = false

nodes = [
  {
    count = 1 
     # Single all-in-one node for testing
    role  = ["etcd", "cp", "worker"]  
  },
  {
    count = 1 
    role  = ["worker"]   
  }
]
