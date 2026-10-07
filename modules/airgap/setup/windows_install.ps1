param (
    [string]$serverIP,
    [string]$token,
    [string]$nodeIP,
    [string]$airgapMethod,
    [string]$agentFlags
)

function Get-ValidatedImageTag {
    [CmdletBinding()]
    param()

    # 1. Grab the OS description from the system
    $OSCaption = (Get-CimInstance Win32_OperatingSystem).Caption

    # 2. Evaluate the string and assign the image tag (Strict matching)
    if ($OSCaption -like "*2022*") {
        $ImageTag = "ltsc2022"
    } 
    elseif ($OSCaption -like "*2025*") {
        $ImageTag = "2025"
    }
    elseif ($OSCaption -like "*2019*") {
        $ImageTag = "1809"
    }
    else {
        throw "ERROR: Unsupported operating system version detected ($OSCaption). Valid image tag could not be determined. Exiting script."
    }

    # 3. Check the current directory for files containing the determined image tag
    # Note: Using $PSScriptRoot targets the directory where the script file resides. 
    # Swap to (Get-Location) if you prefer the user's active console directory.
    $MatchingFiles = Get-ChildItem -Path $PSScriptRoot -File -Filter "*$ImageTag*"

    if (-not $MatchingFiles) {
        throw "ERROR: No files containing the image tag '$ImageTag' were found in the directory '$PSScriptRoot'. Exiting script."
    }

    # 4. Output the validated tag so it can be captured outside the function
    return $ImageTag
}

# Create dirs
New-Item -Type Directory C:/etc/rancher/rke2 -Force
New-Item -Type Directory C:/Users/Administrator/rke2-windows-artifacts/ -Force

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
    $ImageTag = Get-ValidatedImageTag
    Write-Host "The validated image tag available for the rest of the script is: $ImageTag" -ForegroundColor Cyan
    Write-Host "Copy tarball artifacts..."
    New-Item -Type Directory C:/var/lib/rancher/rke2/agent/images -Force
    Copy-Item C:/Users/Administrator/rke2-windows-$ImageTag-amd64-images.tar* C:/var/lib/rancher/rke2/agent/images/
}

Copy-Item C:/Users/Administrator/rke2.windows-amd64.tar.gz C:/Users/Administrator/rke2-windows-artifacts/
Copy-Item C:/Users/Administrator/sha256sum-amd64.txt C:/Users/Administrator/rke2-windows-artifacts/

Write-Host "Install rke2 service..."
C:/Users/Administrator/rke2-install.ps1 -ArtifactPath C:/Users/Administrator/rke2-windows-artifacts

Write-Host "Add rke2 service..."
C:/usr/local/bin/rke2.exe agent service --add
Write-Host "Start rke2 service..."
Start-Service rke2

