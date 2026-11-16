$ErrorActionPreference = 'Stop'
$source = $env:TERMIHOMO_UPDATE_SOURCE
$target = $env:TERMIHOMO_UPDATE_TARGET
try {
    $parent = Get-Process -Id ([int]$env:TERMIHOMO_UPDATE_PARENT) -ErrorAction SilentlyContinue
    if ($parent -and -not $parent.WaitForExit(30000)) { throw 'Termihomo did not exit in time.' }

    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TermihomoUpdate {
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern bool MoveFileEx(string source, string target, uint flags);
}
'@
    $lock = [IO.File]::Open("$target.update.lock", 'Open', 'ReadWrite', 'ReadWrite')
    try {
        $lock.Lock(0, 1)
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ine $env:TERMIHOMO_UPDATE_ORIGINAL) {
            throw 'Termihomo changed during the update. Run the command again.'
        }
        # MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH
        if (-not [TermihomoUpdate]::MoveFileEx($source, $target, 9)) {
            throw [ComponentModel.Win32Exception]::new([Runtime.InteropServices.Marshal]::GetLastWin32Error())
        }
    } finally {
        $lock.Dispose()
    }
} catch {
    [Console]::Error.WriteLine("Termihomo update failed: $($_.Exception.Message)")
    exit 1
} finally {
    Remove-Item -LiteralPath $source -ErrorAction SilentlyContinue
}
