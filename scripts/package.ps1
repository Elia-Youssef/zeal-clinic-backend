#Requires -Version 7.0
<#
.SYNOPSIS
    Release builds without make: the Windows app and updater, the Linux cloud server, the update zips and
    the importer.

.DESCRIPTION
    pwsh scripts/package.ps1 -DistDir <dashboard dist> [-OutDir <dir>] [-Version <x.y.z>]
                             [-Targets windows,cloud,importer]

    Mirrors the Makefile targets release, release-cloud, update-zip, update-zip-cloud and legacyimport:
      local/ZealClinic.exe, local/ZealUpdater.exe
          go build -trimpath -ldflags "-s -w -H=windowsgui -X clinic-api/internal/buildmode.Version=<v>"
      local/ZealClinicUpdate-<v>.zip (+ .sha256)
          both executables at the zip root
      cloud/ZealClinicCloud-<v>-linux-amd64
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags cloud -trimpath -ldflags "-s -w -X ...=<v>"
      cloud/ZealClinicUpdate-<v>-linux-amd64.zip (+ .sha256)
          the cloud binary as ZealClinic at the zip root
      tools/ZealLegacyImport.exe
          go build -ldflags "-X clinic-api/internal/buildmode.Version=<v>-dev" ./cmd/legacyimport

    The dashboard build (-DistDir, a folder with index.html) is copied into client/dist first, and one build
    serves both platforms. -Version defaults to the VERSION file. A .sha256 file holds the lowercase hex
    digest of its zip without a newline, like the Makefile writes it. manifest.json in -OutDir lists every
    output with its size and SHA-256; the last output line is a short JSON summary.

    -OutDir defaults to "package" next to the checkout (RUNNER_TEMP/package on GitHub Actions). It must be
    new or empty and outside the checkout's build/ folder, where the Makefile writes release outputs.

    Release pre-build check: before building, go run ./cmd/releasecheck checks the config overrides of the
    nodes being built (clinic for windows and importer, cloud for cloud), as make release and make
    release-cloud do: each override exists, holds no dev value, a JWT secret of 32 characters or more and a
    64-hex DB key, and the clinic and cloud share the sync secret and the DB key. Messages name keys only.

    Safety: the same checkout guard as scripts/ci.ps1 (marker file or GitHub Actions; internal/config must
    hold exactly the committed files). The builds embed throwaway release values, written into the
    git-ignored override files (random secrets, one sync secret and one database key for both builds, the
    committed ports, an empty PEER_URL, a loopback PUBLIC_URL); the files are removed right after. Go runs
    with HTTP_PROXY and HTTPS_PROXY pointing at a closed local port once the modules are downloaded.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$DistDir,
    [string]$OutDir,
    [string]$Version,
    [ValidateSet('windows', 'cloud', 'importer')][string[]]$Targets = @('windows', 'cloud', 'importer')
)

# Guard: must run before anything else
$RepoRoot = Split-Path -Parent $PSScriptRoot
if ($env:GITHUB_ACTIONS -ne 'true' -and -not (Test-Path -LiteralPath (Join-Path $RepoRoot '.ci-scratch') -PathType Leaf)) {
    [Console]::Error.WriteLine("Refusing to run: '$RepoRoot' has no .ci-scratch marker at its root and this is not a GitHub Actions runner. Packaging changes the checkout it runs in, so run it only in a throwaway clone, with an empty .ci-scratch file created at its root to confirm.")
    exit 1
}
$global:LASTEXITCODE = 99
$configExtra = 'unknown'
try {
    $configExtra = (& git -C $RepoRoot status --porcelain --ignored --untracked-files=all -- internal/config 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -eq 0) { $null = & git -C $RepoRoot diff --quiet HEAD -- internal/config 2>$null }
} catch { $global:LASTEXITCODE = 99 }
if ($LASTEXITCODE -ne 0 -or $configExtra) {
    [Console]::Error.WriteLine('Refusing to run: files under internal/config differ from the committed version or include files git does not track, such as a local.env or cloud.env override (or git could not check them). Local config must never be built, tested or embedded; use a clean checkout.')
    exit 1
}

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem

$OnActions = $env:GITHUB_ACTIONS -eq 'true'
$JailProxy = 'http://127.0.0.1:9'
# The git-ignored override file of each build; its committed dev defaults are <file>.defaults.
$ConfigFiles = [ordered]@{ default = 'internal/config/local.env'; cloud = 'internal/config/cloud.env' }
$ScratchRoot = if ($OnActions -and $env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { Split-Path -Parent $RepoRoot }
$NodesDir = Join-Path $ScratchRoot 'nodes'
$VersionFlag = 'clinic-api/internal/buildmode.Version'

function Test-PathUnder([string]$Path, [string]$Root) {
    $full = [System.IO.Path]::GetFullPath($Path)
    $r = [System.IO.Path]::GetFullPath($Root).TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    return $full.StartsWith($r, [System.StringComparison]::OrdinalIgnoreCase)
}

# Helpers shared with scripts/ci.ps1 and scripts/stack.ps1 (keep in step)

function New-RandomToken([int]$Length) {
    $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
    $chars = [char[]]::new($Length)
    for ($i = 0; $i -lt $Length; $i++) {
        $chars[$i] = $alphabet[[System.Security.Cryptography.RandomNumberGenerator]::GetInt32($alphabet.Length)]
    }
    [string]::new($chars)
}

function New-RandomHexKey {
    <# A random database key: 32 bytes as 64 lowercase hex characters. #>
    $bytes = [byte[]]::new(32)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    [Convert]::ToHexString($bytes).ToLowerInvariant()
}

function Get-DefaultsLayout([string]$Rel) {
    <# Key names (in file order) and the PORT of the committed dev defaults of an override file. #>
    $path = Join-Path $RepoRoot "$Rel.defaults"
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "missing $Rel.defaults" }
    $keys = [System.Collections.Generic.List[string]]::new()
    $port = $null
    foreach ($line in [System.IO.File]::ReadAllLines($path)) {
        if ($line -notmatch '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') { continue }
        $keys.Add($Matches[1])
        if ($Matches[1] -eq 'PORT') { $port = $Matches[2].Trim() }
    }
    [pscustomobject]@{ Keys = $keys.ToArray(); Port = $port }
}

function Write-TestConfig {
    <#
      Writes throwaway release values into the git-ignored override file of both builds (local.env, cloud.env),
      key by key after the committed dev defaults: random secrets, one sync secret and one database key for both
      builds, the committed ports, an empty PEER_URL, a loopback PUBLIC_URL. Restore-CommittedConfig removes them.
    #>
    $sync = New-RandomToken 48
    $publish = New-RandomToken 48
    $dbKey = New-RandomHexKey
    $masks = [System.Collections.Generic.List[string]]::new()
    $masks.Add($sync); $masks.Add($publish); $masks.Add($dbKey)
    foreach ($tag in $ConfigFiles.Keys) {
        $rel = $ConfigFiles[$tag]
        $layout = Get-DefaultsLayout $rel
        $filePort = $layout.Port
        $jwt = New-RandomToken 64
        $masks.Add($jwt)
        $out = [System.Collections.Generic.List[string]]::new()
        foreach ($key in $layout.Keys) {
            $value = $null
            if ($key -eq 'JWT_SECRET') { $value = $jwt }
            elseif ($key -eq 'SYNC_SECRET') { $value = $sync }
            elseif ($key -eq 'PUBLISH_SECRET') { $value = $publish }
            elseif ($key -eq 'DB_ENCRYPTION_KEY') { $value = $dbKey }
            elseif ($key -match 'SECRET$') { $value = New-RandomToken 48; $masks.Add($value) }
            elseif ($key -eq 'PEER_URL') { $value = '' }
            elseif ($key -eq 'PUBLIC_URL') { $value = "http://127.0.0.1:$filePort" }
            elseif ($key -match '(_URL|_DSN)$') { $value = '' }
            if ($null -ne $value) { $out.Add("$key=$value") }
        }
        [System.IO.File]::WriteAllText((Join-Path $RepoRoot $rel), (($out -join "`n") + "`n"), [System.Text.UTF8Encoding]::new($false))
    }
    if ($OnActions) { foreach ($m in $masks) { Write-Host "::add-mask::$m" } }
    Write-Host 'config: throwaway release values written to both override files (committed ports, empty PEER_URL)'
}

function Restore-CommittedConfig {
    <# Removes the override files and checks that internal/config holds exactly the committed files again. #>
    $ok = $true
    foreach ($rel in $ConfigFiles.Values) {
        $path = Join-Path $RepoRoot $rel
        try { if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force } } catch { $ok = $false }
    }
    $global:LASTEXITCODE = 99
    $extra = (& git -C $RepoRoot status --porcelain --ignored --untracked-files=all -- internal/config 2>$null | Out-String).Trim()
    $ok = $ok -and ($LASTEXITCODE -eq 0) -and -not $extra
    $global:LASTEXITCODE = 99
    $null = & git -C $RepoRoot diff --quiet HEAD -- internal/config 2>$null
    $ok = $ok -and ($LASTEXITCODE -eq 0)
    if ($ok) { Write-Host 'config: override files removed, internal/config as committed' }
    else { [Console]::Error.WriteLine('ERROR: internal/config could not be brought back to the committed files; discard this checkout.') }
    return $ok
}

$script:SavedEnv = $null
function Enter-GoJail {
    $goCache = (& go env GOCACHE | Out-String).Trim()
    $names = 'HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY', 'http_proxy', 'https_proxy', 'no_proxy',
    'GOFLAGS', 'GOPROXY', 'GOCACHE', 'LOCALAPPDATA', 'TEMP', 'TMP', 'TMPDIR'
    $script:SavedEnv = [ordered]@{}
    foreach ($n in $names) { $script:SavedEnv[$n] = [Environment]::GetEnvironmentVariable($n) }
    $local = Join-Path $NodesDir 'localappdata'
    $tmp = Join-Path $NodesDir 'tmp'
    New-Item -ItemType Directory -Force -Path $local, $tmp | Out-Null
    foreach ($n in 'NO_PROXY', 'no_proxy') { [Environment]::SetEnvironmentVariable($n, $null) }
    $proxyNames = if ($IsWindows) { @('HTTP_PROXY', 'HTTPS_PROXY') } else { @('HTTP_PROXY', 'HTTPS_PROXY', 'http_proxy', 'https_proxy') }
    foreach ($n in $proxyNames) { [Environment]::SetEnvironmentVariable($n, $JailProxy) }
    $env:GOFLAGS = '-mod=readonly'
    $env:GOPROXY = 'off'
    if ($goCache) { $env:GOCACHE = $goCache }   # keep the shared build cache when LOCALAPPDATA moves
    $env:LOCALAPPDATA = $local
    if ($IsWindows) { $env:TEMP = $tmp; $env:TMP = $tmp } else { $env:TMPDIR = $tmp }
    Write-Host "jail: HTTP(S)_PROXY=$JailProxy, NO_PROXY unset, GOPROXY=off, GOFLAGS=-mod=readonly; LOCALAPPDATA and TEMP under $NodesDir"
}

function Exit-GoJail {
    if ($null -eq $script:SavedEnv) { return }
    foreach ($k in @($script:SavedEnv.Keys)) { [Environment]::SetEnvironmentVariable($k, $script:SavedEnv[$k]) }
    $script:SavedEnv = $null
}

# Packaging

function Invoke-GoBuild([string[]]$Arguments, [hashtable]$ExtraEnv = @{}) {
    $saved = @{}
    foreach ($k in $ExtraEnv.Keys) { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $ExtraEnv[$k]) }
    try {
        $note = if ($ExtraEnv.Count) { '   [' + (($ExtraEnv.GetEnumerator() | Sort-Object Key | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join ' ') + ']' } else { '' }
        Write-Host "> go $($Arguments -join ' ')$note"
        $global:LASTEXITCODE = 99
        & go @Arguments | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "go $($Arguments[0]) failed (exit $LASTEXITCODE)" }
    } finally {
        foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
    }
}

function Get-FileSha256([string]$Path) {
    (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function New-UpdateZip([string]$ZipPath, [System.Collections.IDictionary]$Entries) {
    <# Entries: name in the zip -> source file. Writes the zip and its .sha256 file. #>
    $zip = [System.IO.Compression.ZipFile]::Open($ZipPath, [System.IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($name in $Entries.Keys) {
            $null = [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $Entries[$name], $name,
                [System.IO.Compression.CompressionLevel]::Optimal)
        }
    } finally {
        $zip.Dispose()
    }
    [System.IO.File]::WriteAllText("$ZipPath.sha256", (Get-FileSha256 $ZipPath), [System.Text.Encoding]::ASCII)
    Write-Host "zip: $(Split-Path -Leaf $ZipPath) ($(@($Entries.Keys) -join ', ')) and .sha256"
}

function Copy-DistIntoClient {
    $src = [System.IO.Path]::GetFullPath($DistDir)
    if (-not (Test-Path -LiteralPath (Join-Path $src 'index.html') -PathType Leaf)) { throw "-DistDir has no index.html: $src" }
    $dst = Join-Path $RepoRoot 'client/dist'
    New-Item -ItemType Directory -Force -Path $dst | Out-Null
    Copy-Item -Path (Join-Path $src '*') -Destination $dst -Recurse -Force
    Write-Host "client/dist: copied from $src"
}

# Main

Push-Location $RepoRoot
$exitCode = 1
try {
    if (-not $Version) { $Version = ([System.IO.File]::ReadAllText((Join-Path $RepoRoot 'VERSION'))).Trim() }
    if ($Version -notmatch '^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$') { throw "invalid version '$Version'" }
    if (-not $OutDir) { $OutDir = Join-Path $ScratchRoot 'package' }
    $OutDir = [System.IO.Path]::GetFullPath($OutDir)
    if (Test-PathUnder $OutDir (Join-Path $RepoRoot 'build')) { throw "-OutDir must not be inside the checkout's build/ folder: $OutDir" }
    if ((Test-Path -LiteralPath $OutDir) -and @(Get-ChildItem -LiteralPath $OutDir -Force).Count) { throw "-OutDir is not empty: $OutDir" }
    $local = Join-Path $OutDir 'local'
    $cloud = Join-Path $OutDir 'cloud'
    $tools = Join-Path $OutDir 'tools'
    New-Item -ItemType Directory -Force -Path $local, $cloud, $tools | Out-Null
    Write-Host "package: version $Version, targets $($Targets -join ', '), output $OutDir"

    Write-Host '> go mod download   (before the network jail)'
    $global:LASTEXITCODE = 99
    & go mod download | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "go mod download failed (exit $LASTEXITCODE)" }
    Copy-DistIntoClient

    $releaseFlags = "-s -w -X $VersionFlag=$Version"
    $windowsEnv = if ($IsWindows) { @{} } else { @{ GOOS = 'windows'; GOARCH = 'amd64' } }
    $exe = if ($IsWindows) { '.exe' } else { '' }
    $restored = $false
    Write-TestConfig
    try {
        Enter-GoJail
        # The release pre-build check of the Makefile, for the nodes whose config the targets embed.
        $checkNodes = @()
        if ($Targets -contains 'windows' -or $Targets -contains 'importer') { $checkNodes += 'clinic' }
        if ($Targets -contains 'cloud') { $checkNodes += 'cloud' }
        Invoke-GoBuild (@('run', './cmd/releasecheck') + $checkNodes)
        if ($Targets -contains 'windows') {
            $app = Join-Path $local 'ZealClinic.exe'
            $updater = Join-Path $local 'ZealUpdater.exe'
            Invoke-GoBuild @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui -X $VersionFlag=$Version", '-o', $app, './cmd/server') $windowsEnv
            Invoke-GoBuild @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui -X $VersionFlag=$Version", '-o', $updater, './cmd/updater') $windowsEnv
        }
        if ($Targets -contains 'cloud') {
            $server = Join-Path $cloud "ZealClinicCloud-$Version-linux-amd64"
            Invoke-GoBuild @('build', '-tags', 'cloud', '-trimpath', '-ldflags', $releaseFlags, '-o', $server, './cmd/server') @{ CGO_ENABLED = '0'; GOOS = 'linux'; GOARCH = 'amd64' }
        }
        if ($Targets -contains 'importer') {
            Invoke-GoBuild @('build', '-ldflags', "-X $VersionFlag=$Version-dev", '-o', (Join-Path $tools "ZealLegacyImport$exe"), './cmd/legacyimport')
        }
    } finally {
        Exit-GoJail
        $restored = Restore-CommittedConfig
    }
    if (-not $restored) { throw 'config restore failed' }

    if ($Targets -contains 'windows') {
        New-UpdateZip (Join-Path $local "ZealClinicUpdate-$Version.zip") ([ordered]@{
                'ZealClinic.exe' = (Join-Path $local 'ZealClinic.exe'); 'ZealUpdater.exe' = (Join-Path $local 'ZealUpdater.exe') })
    }
    if ($Targets -contains 'cloud') {
        New-UpdateZip (Join-Path $cloud "ZealClinicUpdate-$Version-linux-amd64.zip") ([ordered]@{
                'ZealClinic' = (Join-Path $cloud "ZealClinicCloud-$Version-linux-amd64") })
    }

    $files = @(Get-ChildItem -LiteralPath $OutDir -Recurse -File | Sort-Object FullName | ForEach-Object {
            [ordered]@{
                path   = [System.IO.Path]::GetRelativePath($OutDir, $_.FullName) -replace '\\', '/'
                bytes  = $_.Length
                sha256 = Get-FileSha256 $_.FullName
            }
        })
    $manifest = [ordered]@{ version = $Version; targets = $Targets; dist = [System.IO.Path]::GetFullPath($DistDir); files = $files }
    $manifestPath = Join-Path $OutDir 'manifest.json'
    [System.IO.File]::WriteAllText($manifestPath, (($manifest | ConvertTo-Json -Depth 5) -replace "`r`n", "`n") + "`n", [System.Text.UTF8Encoding]::new($false))
    foreach ($f in $files) { Write-Host ('{0,12:N0}  {1}  {2}' -f $f.bytes, $f.sha256.Substring(0, 12), $f.path) }
    [Console]::Out.WriteLine((([ordered]@{ version = $Version; outDir = $OutDir; manifest = $manifestPath; files = $files.Count }) | ConvertTo-Json -Compress))
    $exitCode = 0
} catch {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    $exitCode = 1
} finally {
    Pop-Location
}
exit $exitCode
