<powershell>
$ErrorActionPreference = "Stop"
Start-Transcript -Path "$env:ProgramData\ssh_setup.log" -Append

if ((Get-WindowsCapability -Online -Name OpenSSH.Server*).State -ne 'Installed') {
    try {
        Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
    } catch {
        throw "OpenSSH.Server is not in the AMI and cannot be downloaded in the airgapped subnet: $_"
    }
}
Set-Service -Name sshd -StartupType Automatic
Stop-Service sshd -ErrorAction SilentlyContinue
New-NetFirewallRule -Name sshd -DisplayName 'OpenSSH SSH Server' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 -RemoteAddress ("${ssh_allowed_cidrs}" -split ",")

New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell -Value "C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe" -PropertyType String -Force

# Public half of the launch key pair comes from IMDS; no private key is stored on the node.
$adminKeyPath = "$env:ProgramData\ssh\administrators_authorized_keys"
$sshConfigDir = Split-Path $adminKeyPath
if (-not (Test-Path $sshConfigDir)) { New-Item -ItemType Directory -Path $sshConfigDir | Out-Null }

$keyPath = "/latest/meta-data/public-keys/0/openssh-key"
$pubKey = ""
for ($i = 1; $i -le 12 -and $pubKey -notmatch '^ssh-'; $i++) {
    try {
        $pubKey = "$(Get-EC2InstanceMetadata -Path $keyPath)".Trim()
    } catch {
        Write-Output "attempt $i cmdlet lookup failed: $_"
    }
    if ($pubKey -notmatch '^ssh-') {
        try {
            $imdsToken = Invoke-RestMethod -Method Put -Uri "http://instance-data/latest/api/token" -Headers @{"X-aws-ec2-metadata-token-ttl-seconds" = "300"}
            $pubKey = "$(Invoke-RestMethod -Uri "http://instance-data$keyPath" -Headers @{"X-aws-ec2-metadata-token" = $imdsToken})".Trim()
        } catch {
            Write-Output "attempt $i http lookup failed: $_"
        }
    }
    if ($pubKey -notmatch '^ssh-') { Start-Sleep -Seconds 10 }
}
if ($pubKey -notmatch '^ssh-') { throw "public key not available from instance metadata" }
Set-Content -Path $adminKeyPath -Value $pubKey -Encoding ascii
icacls.exe $adminKeyPath /inheritance:r /grant "Administrators:F" /grant "SYSTEM:F"

# sshd stays stopped until the reboot so a reachable sshd means setup is complete.
Enable-WindowsOptionalFeature -Online -FeatureName Containers -All -NoRestart
Restart-Computer -Force
</powershell>
