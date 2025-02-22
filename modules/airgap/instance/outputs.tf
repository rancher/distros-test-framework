output "bastion_ip" {
  value = aws_instance.bastion[0].public_ip
  description = "The public IP of the AWS bastion node"
  depends_on = [ aws_instance.bastion ]
}

output "bastion_dns" {
  value = aws_instance.bastion[0].public_dns
  description = "The public DNS of the AWS bastion node"
  depends_on = [ aws_instance.bastion ]
}

output "master_ips" {
  value = join("," ,aws_instance.master.*.private_ip)
  description = "The private IP of the AWS private master node"
  depends_on = [ aws_instance.master ]
}

output "worker_ips" {
  value = join("," ,aws_instance.worker.*.private_ip)
  description = "The private IP of the AWS private worker node"
  depends_on = [ aws_instance.worker ]
}

output "windows_worker_ips" {
  value = join("," ,aws_instance.windows_worker.*.private_ip)
  description = "The private IP of the AWS private worker node"
  depends_on = [ aws_instance.worker ]
}

output "windows_worker_password_decrypted" {
  value = [
    for agent in aws_instance.windows_worker : rsadecrypt(agent.password_data, file(var.access_key))
  ]
}