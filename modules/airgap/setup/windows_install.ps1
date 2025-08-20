param (
    [string]$serverIP,
    [string]$token,
    [string]$nodeIP,
    [string]$airgapMethod,
    [string]$agentFlags
)

# Create dirs
New-Item -Type Directory C:/etc/rancher/rke2 -Force

# Setting config
Write-Host "Set config.yaml..."
Set-Content -Path C:/etc/rancher/rke2/config.yaml -Value @"
server: "https://$($serverIP):9345"
token: "$($token)"
node-ip: "$($nodeIP)"
"@
if (!$agentFlags) {
    Add-Content -Path C:/etc/rancher/rke2/config.yaml -Value "$($agentFlags)"
}

# Setting env
Write-Host "Set env path..."
$env:PATH+=";c:\var\lib\rancher\rke2\bin;c:\usr\local\bin"
[Environment]::SetEnvironmentVariable(
    "Path",
    [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine) + ";c:\var\lib\rancher\rke2\bin;c:\usr\local\bin;c:\var\lib\rancher\rke2\",
    [EnvironmentVariableTarget]::Machine)

# Checking airgap method
if ($airgapMethod -like "private_registry") {
    Write-Host "Copy registries.yaml..."
    Copy-Item C:/Users/Administrator/registries-windows.yaml C:/etc/rancher/rke2/registries.yaml
}
if ($airgapMethod -like "tarball") {
    Write-Host "Copy tarball artifacts..."
    New-Item -Type Directory C:/var/lib/rancher/rke2/agent/images -Force
    Copy-Item C:/Users/Administrator/rke2-windows-ltsc2022-amd64-images.tar* C:/var/lib/rancher/rke2/agent/images/
    # New-Item -Type Directory C:/Users/Administrator/rke2-windows-artifacts
    # TODO: for windows 2019
    #Copy-Item C:/Users/Administrator/rke2-windows-1809-amd64-images.tar* C:/var/lib/rancher/rke2/agent/images/
}

# Install rke2 service
Write-Host "Install rke2 service..."
C:/Users/Administrator/rke2-install.ps1 -ArtifactPath C:/Users/Administrator/rke2-windows-artifacts

# Starting rke2 service
Write-Host "Add rke2 service..."
C:/usr/local/bin/rke2.exe agent service --add
Write-Host "Start rke2 service..."
Start-Service rke2

