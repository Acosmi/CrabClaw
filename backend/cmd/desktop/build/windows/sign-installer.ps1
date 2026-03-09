param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactPath,

    [Parameter(Mandatory = $true)]
    [string]$CertificateBase64,

    [Parameter(Mandatory = $true)]
    [string]$CertificatePassword,

    [string]$TimestampUrl = "",

    [string]$StatusPath = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-SignToolPath {
    $command = Get-Command signtool.exe -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($command) {
        return $command.Source
    }

    $roots = @(
        "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
        "${env:ProgramFiles}\Windows Kits\10\bin"
    )

    $candidates = foreach ($root in $roots) {
        if (-not [string]::IsNullOrWhiteSpace($root) -and (Test-Path $root)) {
            Get-ChildItem -Path $root -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue
        }
    }

    $selected = $candidates |
        Sort-Object FullName -Descending |
        Select-Object -First 1

    if (-not $selected) {
        throw "signtool.exe not found on the current runner"
    }

    return $selected.FullName
}

if (-not (Test-Path $ArtifactPath)) {
    throw "Installer not found: $ArtifactPath"
}

$signTool = Resolve-SignToolPath
$tempPfx = Join-Path ([System.IO.Path]::GetTempPath()) ("desktop-release-" + [System.Guid]::NewGuid().ToString("N") + ".pfx")

try {
    [System.IO.File]::WriteAllBytes($tempPfx, [System.Convert]::FromBase64String($CertificateBase64))

    $arguments = @(
        "sign",
        "/fd", "SHA256",
        "/f", $tempPfx,
        "/p", $CertificatePassword
    )
    if (-not [string]::IsNullOrWhiteSpace($TimestampUrl)) {
        $arguments += @("/tr", $TimestampUrl, "/td", "SHA256")
    }
    $arguments += $ArtifactPath

    & $signTool @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "signtool exited with code $LASTEXITCODE"
    }

    $signature = Get-AuthenticodeSignature -FilePath $ArtifactPath
    if ($signature.Status -ne "Valid") {
        throw "signature verification failed with status $($signature.Status)"
    }

    if ($StatusPath) {
        @(
            "Windows installer signed successfully.",
            "signtool: $signTool",
            "signature-status: $($signature.Status)",
            "timestamp-url: $TimestampUrl"
        ) | Set-Content -Path $StatusPath
    }
}
finally {
    if (Test-Path $tempPfx) {
        Remove-Item -Force $tempPfx
    }
}
