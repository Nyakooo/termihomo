$ErrorActionPreference = 'Stop'
$repo = 'Nyakooo/termihomo'
$api = "https://api.github.com/repos/$repo"
$headers = @{ Accept = 'application/vnd.github+json' }
$token = if ($env:GH_TOKEN) { $env:GH_TOKEN } else { $env:GITHUB_TOKEN }
if ($token) { $headers.Authorization = "Bearer $token" }

$machine = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($machine) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw 'Termihomo requires Windows AMD64 or ARM64.' }
}
$name = "termihomo-windows-$arch.exe"
try {
    $release = Invoke-RestMethod -Uri "$api/releases/latest" -Headers $headers
} catch {
    if ($token -or $_.Exception.Response.StatusCode -ne 404 -or -not (Get-Command gh -ErrorAction SilentlyContinue)) {
        throw
    }
    $token = gh auth token
    if ($LASTEXITCODE -ne 0 -or -not $token) { throw 'Could not read the GitHub CLI token.' }
    $headers.Authorization = "Bearer $token"
    $release = Invoke-RestMethod -Uri "$api/releases/latest" -Headers $headers
}
if ($release.tag_name -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') {
    throw 'GitHub did not return a stable Termihomo release.'
}

function Get-ReleaseFile($Name, $Path) {
    $files = @($release.assets | Where-Object { $_.name -ceq $Name })
    if ($files.Count -ne 1) { throw "Expected one release asset named $Name." }
    if ($token) {
        Invoke-WebRequest -UseBasicParsing -Uri "$api/releases/assets/$($files[0].id)" -Headers @{
            Authorization = "Bearer $token"; Accept = 'application/octet-stream'
        } -OutFile $Path
    } else {
        $uri = "https://github.com/$repo/releases/download/$($release.tag_name)/$Name"
        Invoke-WebRequest -UseBasicParsing -Uri $uri -OutFile $Path
    }
}

$dest = if ($env:TERMIHOMO_INSTALL_DIR) { $env:TERMIHOMO_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\Termihomo' }
$dest = [IO.Path]::GetFullPath($dest)
[void][IO.Directory]::CreateDirectory($dest)
$target = Join-Path $dest 'termihomo.exe'
$suffix = [Guid]::NewGuid().ToString('N')
$staged = Join-Path $dest ".termihomo-$suffix.exe"
$sums = Join-Path $dest ".termihomo-$suffix.sha256"
$backup = Join-Path $dest ".termihomo-$suffix.old"
try {
    Get-ReleaseFile $name $staged
    Get-ReleaseFile 'SHA256SUMS' $sums
    $pattern = '^([A-Fa-f0-9]{64})\s+\*?' + [Regex]::Escape($name) + '$'
    $entries = @(Get-Content -LiteralPath $sums | Select-String -Pattern $pattern -CaseSensitive)
    if ($entries.Count -ne 1) { throw 'Invalid SHA256SUMS.' }
    $expected = $entries[0].Matches[0].Groups[1].Value
    if ((Get-FileHash -LiteralPath $staged -Algorithm SHA256).Hash -ine $expected) {
        throw 'Termihomo SHA-256 verification failed.'
    }
    if (Test-Path -LiteralPath $target) {
        [IO.File]::Replace($staged, $target, $backup)
        Remove-Item -LiteralPath $backup
    } else {
        [IO.File]::Move($staged, $target)
    }
} finally {
    foreach ($path in @($staged, $sums)) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path }
    }
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dest) {
    $paths = @($userPath, $dest) | Where-Object { $_ }
    [Environment]::SetEnvironmentVariable('Path', ($paths -join ';'), 'User')
}
if (($env:Path -split ';') -notcontains $dest) { $env:Path += ";$dest" }
Write-Output "Installed Termihomo $($release.tag_name) at $target"
