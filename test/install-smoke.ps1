param([Parameter(Mandatory)][string]$Asset)

$ErrorActionPreference = 'Stop'
$assetFile = Get-Item -LiteralPath $Asset
$fixture = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
$previousPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$previousInstallDir = $env:TERMIHOMO_INSTALL_DIR
$env:TERMIHOMO_INSTALL_DIR = Join-Path $fixture 'install'
$null = New-Item -ItemType Directory -Path $fixture
try {
    $payload = Join-Path $fixture $assetFile.Name
    $manifest = Join-Path $fixture 'SHA256SUMS'
    Copy-Item -LiteralPath $assetFile.FullName -Destination $payload
    $hash = (Get-FileHash -LiteralPath $payload).Hash
    Set-Content -LiteralPath $manifest -Value "$hash  $($assetFile.Name)" -Encoding ASCII
    function Invoke-RestMethod {
        @{ tag_name = 'v1.2.3'; assets = @(@{ name = $assetFile.Name; id = 1 }, @{ name = 'SHA256SUMS'; id = 2 }) }
    }
    function Invoke-WebRequest {
        param([switch]$UseBasicParsing, [string]$Uri, $Headers, [string]$OutFile)
        $file = if ($Uri -match '/(2|SHA256SUMS)$') { $manifest } else { $payload }
        Copy-Item -LiteralPath $file -Destination $OutFile
    }
    & ./install.ps1
    & ./install.ps1
    $installed = Join-Path $env:TERMIHOMO_INSTALL_DIR 'termihomo.exe'
    if ((Get-FileHash -LiteralPath $installed).Hash -ne $hash) { throw 'Installed executable differs.' }
    Set-Content -LiteralPath $payload -Value 'invalid binary'
    try {
        & ./install.ps1
        throw 'Installer accepted a checksum mismatch.'
    } catch {
        if ($_.Exception.Message -ne 'Termihomo SHA-256 verification failed.') { throw }
    }
    if ((Get-FileHash -LiteralPath $installed).Hash -ne $hash) { throw 'Failed install changed the executable.' }
} finally {
    [Environment]::SetEnvironmentVariable('Path', $previousPath, 'User')
    $env:TERMIHOMO_INSTALL_DIR = $previousInstallDir
    Remove-Item -LiteralPath $fixture -Recurse -Force
}
