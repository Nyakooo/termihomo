$ErrorActionPreference = 'Stop'
$target = $env:TERMIHOMO_REMOVE_TARGET
try {
    $parent = Get-Process -Id ([int]$env:TERMIHOMO_REMOVE_PARENT) -ErrorAction SilentlyContinue
    if ($parent -and -not $parent.WaitForExit(30000)) { throw 'Termihomo did not exit in time.' }

    $lockPath = "$target.update.lock"
    $lock = [IO.File]::Open($lockPath, 'Open', 'ReadWrite', 'ReadWrite')
    try {
        $lock.Lock(0, 1)
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ine $env:TERMIHOMO_REMOVE_HASH) {
            throw 'Termihomo changed during uninstall.'
        }
        Remove-Item -LiteralPath $target
    } finally {
        $lock.Dispose()
    }
    Remove-Item -LiteralPath $lockPath

    $dir = [IO.Path]::GetDirectoryName($target)
    $defaultDir = Join-Path $env:LOCALAPPDATA 'Programs\Termihomo'
    if ($dir -ieq $defaultDir -and -not (Get-ChildItem -Force -LiteralPath $dir | Select-Object -First 1)) {
        Remove-Item -LiteralPath $dir
        $path = [Environment]::GetEnvironmentVariable('Path', 'User')
        if ($path) {
            $entries = $path -split ';' | Where-Object { $_ -ine $dir }
            [Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
        }
    }
} catch {
    [Console]::Error.WriteLine("Termihomo uninstall failed: $($_.Exception.Message)")
    exit 1
}
