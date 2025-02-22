New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell -Value "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" -PropertyType String -Force
New-Item -Type Directory C:/etc/rancher/rke2 -Force
New-Item -Type Directory C:/var/lib/rancher/rke2/agent/images/ -Force
