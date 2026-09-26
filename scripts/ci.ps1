#Requires -Version 7.0
<#
.SYNOPSIS
    Backend CI stages, shared by local runs and GitHub Actions.

.DESCRIPTION
    pwsh scripts/ci.ps1 -Stage <name> [-Tags default|cloud] [-ArtifactsDir <dir>] [-Shuffle] [-Packages <list>]
                        [-DashboardDir <dir>]

    Stages
      env-check  the committed config holds dev values only: no internal/config/*.env file is tracked, the
                 *.env.defaults files keep every *SECRET key on a dev-only- value, a 64-hex DB key shared by
                 both builds (like SYNC_SECRET) and an empty PEER_URL and PUBLIC_URL, the *.env.example files
                 list the same keys with empty values, and git ignores local.env and cloud.env (key and file
                 names only are printed)
      platform   the cloud build does not depend on the tray library; the clinic server is Windows-only
      vet        go vet for the tag (cloud: natively and for linux; default: natively on Windows)
      go-test    go test -count=1 -json for the tag; the JSON stream and per-package counts go to -ArtifactsDir
      fmt        gofmt over the tracked Go files (client/dist excluded), as a ratchet on
                 scripts/baseline/gofmt.json: a new unformatted file fails
      mod        go mod verify and go mod tidy -diff must both be clean. Builds and runs nothing; tidy also
                 resolves the test imports of dependencies, so it uses the module proxy (not jailed).
      vuln       govulncheck for both tags, or the one given with -Tags; the clinic build is analysed for
                 windows, the cloud build for linux. Reachable findings are a ratchet on
                 scripts/baseline/govulncheck.json: a new ID fails. Needs network for the vulnerability
                 database, so this stage is not jailed.
      secrets    gitleaks (always --redact) with .gitleaks.toml, inside the jail: the committed tree, and
                 the git history as a report only. Findings are compared by fingerprint (rule, file, line;
                 commit for history) with scripts/baseline/gitleaks.json: a new tree finding fails. Only
                 fingerprints are printed or written, never the matched text.
      The scanners are pinned below and built with go install into the scratch area.
      system     builds a clinic node (port 55581, PEER_URL on the test proxy at 55580), a cloud node (55582)
                 and a cloud stamped with another version (55555) with scripts/stack.ps1 -Build, sharing one
                 random sync secret and database key, then runs go test -tags systest ./systest/... against them. Binaries
                 go to -BinDir (keep it stable: Windows Firewall asks once per binary path); -Full adds the
                 long-running checks. The go test JSON, a per-step table and every node log go to
                 -ArtifactsDir.
      package    scripts/package.ps1 with -DistDir (a placeholder page without it) into the scratch area (its
                 release pre-build check passes on generated values), then a smoke check: the Windows exe
                 seeds and migrates a database in an installed layout
                 (--seed-only --no-browser, LOCALAPPDATA with Zeal Clinic\Data), the executables have the
                 expected subsystem and build settings, the Linux binary is static x86-64, the zips hold
                 the expected files and match their .sha256. Sizes go to -ArtifactsDir; binaries do not.
      contract   the API contract goldens and the dashboard parity tests (TestContract, TestParity*) in
                 internal/api/server for the tag, with DASHBOARD_DIR set to -DashboardDir (without it the parity
                 tests skip); same config, jail and report as go-test. The same tests also run in go-test, where
                 the parity tests skip.

    Ratchets print "can tighten" for baseline entries that are gone, and write the current state in the
    baseline format to -ArtifactsDir, ready to replace the baseline file after review.

    Safety
      - Runs only in a disposable checkout: a GitHub Actions runner, or a throwaway clone marked by an
        empty `.ci-scratch` file at its root (create it there to confirm that the stages may change that
        clone; never in a working copy). Anything else is refused.
      - Refuses when files under internal/config differ from the committed version or a file that git
        does not track is there (such as a local.env or cloud.env override). The config files are
        embedded into every build, so local config must never be built, tested or run.
      - Stages that compile Go code write throwaway test values into the git-ignored override files of both
        builds (random secrets, one sync secret and one database key for both, loopback URLs, the
        committed ports unless -Port is given) and remove them afterwards. The values exist only in
        memory and in this checkout.
      - Go commands run with HTTP_PROXY/HTTPS_PROXY pointing at a closed local port (127.0.0.1:9),
        NO_PROXY unset, GOPROXY=off and -mod=readonly: an outbound HTTP call fails at once while loopback
        traffic is unaffected. Modules are downloaded first, before the jail.
      - LOCALAPPDATA and TEMP point into the scratch area while Go runs; builds are never version-stamped.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('env-check', 'platform', 'vet', 'go-test', 'fmt', 'mod', 'contract', 'vuln', 'secrets', 'package', 'system', 'workflows')]
    [string]$Stage,
    [ValidateSet('default', 'cloud')]
    [string]$Tags = 'default',
    [string]$ArtifactsDir,
    [switch]$Shuffle,
    # Package patterns for vet and go-test (comma or space separated). Default: ./...
    [string[]]$Packages,
    # PORT for the active tag's config file. 0 keeps the committed value.
    [int]$Port = 0,
    # PEER_URL for the clinic config (127.0.0.1 or localhost only). Empty by default.
    [string]$PeerUrl = '',
    # system: folder for the node binaries. Default: nodes/bin in the scratch area.
    [string]$BinDir,
    # system: also run the long-running checks.
    [switch]$Full,
    # package: the dashboard build to embed. Default: a one-page placeholder.
    [string]$DistDir,
    # Dashboard checkout for the contract stage's parity tests.
    [string]$DashboardDir = ''
)

# Guard: must run before anything else
$RepoRoot = Split-Path -Parent $PSScriptRoot
if ($env:GITHUB_ACTIONS -ne 'true' -and -not (Test-Path -LiteralPath (Join-Path $RepoRoot '.ci-scratch') -PathType Leaf)) {
    [Console]::Error.WriteLine("Refusing to run: '$RepoRoot' has no .ci-scratch marker at its root and this is not a GitHub Actions runner. The stages change the checkout they run in, so run them only in a throwaway clone, with an empty .ci-scratch file created at its root to confirm.")
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

$OnActions = $env:GITHUB_ACTIONS -eq 'true'
$ScratchRoot = if ($OnActions -and $env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { Split-Path -Parent $RepoRoot }
$NodesDir = Join-Path $ScratchRoot 'nodes'
if (-not $ArtifactsDir) { $ArtifactsDir = Join-Path $ScratchRoot (Join-Path 'artifacts' "backend-$Stage-$Tags") }
$ArtifactsDir = [System.IO.Path]::GetFullPath($ArtifactsDir)
$JailProxy = 'http://127.0.0.1:9'
# The git-ignored override file of each build; its committed dev defaults are <file>.defaults.
$ConfigFiles = [ordered]@{ default = 'internal/config/local.env'; cloud = 'internal/config/cloud.env' }
$Pkgs = @('./...')
if ($Packages) { $Pkgs = @($Packages -split '[,\s]+' | Where-Object { $_ }) }
$PortOverrides = @{}
if ($Port -gt 0) { $PortOverrides[$Tags] = $Port }
$TagsGiven = $PSBoundParameters.ContainsKey('Tags')
$BaselineDir = Join-Path $RepoRoot 'scripts/baseline'
# Pinned tool versions; bump deliberately.
$GovulncheckModule = 'golang.org/x/vuln/cmd/govulncheck@v1.8.0'
$GitleaksModule = 'github.com/zricethezav/gitleaks/v8@v8.30.1'

if ($Port -ne 0 -and ($Port -lt 1024 -or $Port -gt 65535)) {
    [Console]::Error.WriteLine("Invalid -Port $Port (use 1024-65535, or 0 for the committed value)")
    exit 1
}
if ($PeerUrl) {
    $u = $null
    if (-not [Uri]::TryCreate($PeerUrl, [UriKind]::Absolute, [ref]$u) -or $u.Scheme -notin 'http', 'https' -or $u.Host -notin '127.0.0.1', 'localhost') {
        [Console]::Error.WriteLine('Refusing -PeerUrl: only http(s) URLs on 127.0.0.1 or localhost are allowed (a dev peer listens on 127.0.0.1 only).')
        exit 1
    }
}

$PendingStages = @()
if ($PendingStages -contains $Stage) {
    Write-Host "Pending: stage '$Stage' is not implemented yet"
    exit 0
}

New-Item -ItemType Directory -Force -Path $ArtifactsDir | Out-Null

# Helpers

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

function Write-TestConfig([hashtable]$PortOverrides = @{}, [string]$Peer = '') {
    <#
      Writes throwaway values into the git-ignored override file of both builds (local.env, cloud.env), key by
      key after the committed dev defaults: random secrets, one sync secret and one database key for both builds,
      the committed ports unless overridden, loopback URLs. Restore-CommittedConfig removes the files again.
    #>
    $sync = New-RandomToken 48
    $publish = New-RandomToken 48
    $dbKey = New-RandomHexKey
    $masks = [System.Collections.Generic.List[string]]::new()
    $masks.Add($sync); $masks.Add($publish); $masks.Add($dbKey)
    $described = [System.Collections.Generic.List[string]]::new()
    foreach ($tag in $ConfigFiles.Keys) {
        $rel = $ConfigFiles[$tag]
        $layout = Get-DefaultsLayout $rel
        $filePort = if ($PortOverrides.Contains($tag)) { "$($PortOverrides[$tag])" } else { $layout.Port }
        $jwt = New-RandomToken 64
        $masks.Add($jwt)
        $out = [System.Collections.Generic.List[string]]::new()
        foreach ($key in $layout.Keys) {
            $value = $null
            if ($key -eq 'PORT') { $value = $filePort }
            elseif ($key -eq 'JWT_SECRET') { $value = $jwt }
            elseif ($key -eq 'SYNC_SECRET') { $value = $sync }
            elseif ($key -eq 'PUBLISH_SECRET') { $value = $publish }
            elseif ($key -eq 'DB_ENCRYPTION_KEY') { $value = $dbKey }
            elseif ($key -match 'SECRET$') { $value = New-RandomToken 48; $masks.Add($value) }
            elseif ($key -eq 'PEER_URL') { $value = $(if ($tag -eq 'default') { $Peer } else { '' }) }
            elseif ($key -eq 'PUBLIC_URL') { $value = "http://127.0.0.1:$filePort" }
            elseif ($key -match '(_URL|_DSN)$') { $value = '' }
            if ($null -ne $value) { $out.Add("$key=$value") }
        }
        [System.IO.File]::WriteAllText((Join-Path $RepoRoot $rel), (($out -join "`n") + "`n"), [System.Text.UTF8Encoding]::new($false))
        $described.Add("$rel (PORT $filePort)")
    }
    if ($OnActions) { foreach ($m in $masks) { Write-Host "::add-mask::$m" } }
    $peerNote = if ($Peer) { 'loopback PEER_URL' } else { 'empty PEER_URL' }
    Write-Host "config: throwaway test values written to $($described -join ', '); $peerNote"
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

function Initialize-GoModules {
    Write-Host '> go mod download   (before the network jail)'
    $global:LASTEXITCODE = 99
    & go mod download | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "go mod download failed (exit $LASTEXITCODE)" }
}

function Initialize-ClientDist {
    # The server embeds client/dist; the embed directive fails when the folder is missing or empty.
    $dist = Join-Path $RepoRoot 'client/dist'
    $any = (Test-Path -LiteralPath $dist) -and
        ($null -ne (Get-ChildItem -LiteralPath $dist -Recurse -File -Force -ErrorAction SilentlyContinue | Select-Object -First 1))
    if ($any) { return $false }
    New-Item -ItemType Directory -Force -Path $dist | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $dist 'index.html'),
        "<!doctype html><title>Zeal Clinic</title><p>Placeholder: the dashboard is not built in this checkout.</p>`n")
    Write-Host 'client/dist was empty: wrote a one-file placeholder page'
    return $true
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

function Set-TempEnv([hashtable]$Values) {
    $saved = @{}
    foreach ($k in $Values.Keys) { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $Values[$k]) }
    $saved
}

function Format-EnvNote([hashtable]$Values) {
    if (-not $Values -or $Values.Count -eq 0) { return '' }
    '   [' + (($Values.GetEnumerator() | Sort-Object Key | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join ' ') + ']'
}

function Invoke-Go([string[]]$Arguments, [hashtable]$ExtraEnv = @{}) {
    <# Streams go output to the console; returns the exit code. #>
    $saved = Set-TempEnv $ExtraEnv
    try {
        Write-Host ('> go {0}{1}' -f ($Arguments -join ' '), (Format-EnvNote $ExtraEnv))
        $global:LASTEXITCODE = 99
        & go @Arguments | Out-Host
        return $LASTEXITCODE
    } finally {
        $null = Set-TempEnv $saved
    }
}

function Get-GoLines([string[]]$Arguments, [hashtable]$ExtraEnv = @{}) {
    <# Returns stdout lines; the exit code goes to $script:GoExit. #>
    $saved = Set-TempEnv $ExtraEnv
    try {
        Write-Host ('> go {0}{1}' -f ($Arguments -join ' '), (Format-EnvNote $ExtraEnv))
        $global:LASTEXITCODE = 99
        $lines = @(& go @Arguments)
        $script:GoExit = $LASTEXITCODE
        return , $lines
    } finally {
        $null = Set-TempEnv $saved
    }
}

function Invoke-GoToFile([string[]]$Arguments, [string]$OutFile) {
    <# Runs go with stdout written byte for byte to OutFile (stderr stays on the console). #>
    $go = (Get-Command go -CommandType Application | Select-Object -First 1).Source
    $psi = [System.Diagnostics.ProcessStartInfo]::new($go)
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.WorkingDirectory = $RepoRoot
    Write-Host "> go $($Arguments -join ' ')   [stdout: $OutFile]"
    $p = [System.Diagnostics.Process]::Start($psi)
    $fs = [System.IO.File]::Create($OutFile)
    try { $p.StandardOutput.BaseStream.CopyTo($fs) } finally { $fs.Dispose() }
    $p.WaitForExit()
    $code = $p.ExitCode
    $p.Dispose()
    return $code
}

function Write-GoTestReport([string]$JsonPath, [string]$TxtPath, [int]$GoExit) {
    <# Per-package pass/fail/skip counts from a go test -json stream. Returns the summary line. #>
    Set-StrictMode -Off
    $pkgState = [ordered]@{}
    $final = @{}
    $buildFailed = [System.Collections.Generic.List[string]]::new()
    $seeds = [System.Collections.Generic.List[string]]::new()
    $nonJson = 0
    if (Test-Path -LiteralPath $JsonPath) {
        foreach ($line in [System.IO.File]::ReadLines($JsonPath)) {
            if (-not $line.StartsWith('{')) { if ($line.Trim()) { $nonJson++ }; continue }
            if ($line.Contains('"Action":"output"')) {
                if ($line -match '-test\.shuffle (\d+)') { $seeds.Add($Matches[1]) }
                continue
            }
            if ($line.Contains('"Action":"build-output"')) { continue }
            $ev = $line | ConvertFrom-Json
            if ($ev.Action -eq 'build-fail') { if ($ev.ImportPath) { $buildFailed.Add($ev.ImportPath) }; continue }
            $pkg = $ev.Package
            if (-not $pkg) { continue }
            if (-not $pkgState.Contains($pkg)) { $pkgState[$pkg] = [ordered]@{ result = ''; elapsed = 0.0 } }
            if ($ev.Action -notin 'pass', 'fail', 'skip') { continue }
            if ($ev.Test) {
                $final["$pkg`t$($ev.Test)"] = $ev.Action
            } else {
                $pkgState[$pkg].result = $ev.Action
                if ($ev.Elapsed) { $pkgState[$pkg].elapsed = [double]$ev.Elapsed }
                if ($ev.FailedBuild -and -not $buildFailed.Contains($ev.FailedBuild)) { $buildFailed.Add($ev.FailedBuild) }
            }
        }
    }

    $counts = @{}
    $failedTests = [System.Collections.Generic.List[string]]::new()
    foreach ($kv in $final.GetEnumerator()) {
        $pkg, $test = $kv.Key -split "`t", 2
        if (-not $counts.Contains($pkg)) { $counts[$pkg] = @{ pass = 0; fail = 0; skip = 0; spass = 0; sfail = 0; sskip = 0 } }
        $isSub = $test.Contains('/')
        $counts[$pkg][$(if ($isSub) { 's' + $kv.Value } else { $kv.Value })]++
        if ($kv.Value -eq 'fail') { $failedTests.Add("$pkg  $test") }
    }

    $t = @{ pass = 0; fail = 0; skip = 0; spass = 0; sfail = 0; sskip = 0 }
    $pkgOk = 0; $pkgFail = 0; $pkgNoTests = 0
    $rows = [System.Collections.Generic.List[string]]::new()
    $rows.Add(('{0,-48} {1,6} {2,6} {3,6}  {4,-13} {5,8}' -f 'PACKAGE', 'PASS', 'FAIL', 'SKIP', 'RESULT', 'SECONDS'))
    foreach ($pkg in $pkgState.Keys) {
        $c = if ($counts.Contains($pkg)) { $counts[$pkg] } else { @{ pass = 0; fail = 0; skip = 0; spass = 0; sfail = 0; sskip = 0 } }
        foreach ($k in @($t.Keys)) { $t[$k] += $c[$k] }
        $tests = $c.pass + $c.fail + $c.skip
        $res = $pkgState[$pkg].result
        $label = switch ($res) {
            'pass' { 'ok' }
            'fail' { 'FAIL' }
            'skip' { if ($tests -eq 0) { 'no test files' } else { 'skip' } }
            default { 'incomplete' }
        }
        if ($label -eq 'ok') { $pkgOk++ } elseif ($label -eq 'no test files') { $pkgNoTests++ } elseif ($label -ne 'skip') { $pkgFail++ }
        if ($label -eq 'no test files') { continue }
        $rows.Add(('{0,-48} {1,6} {2,6} {3,6}  {4,-13} {5,8:0.0}' -f $pkg, $c.pass, $c.fail, $c.skip, $label, $pkgState[$pkg].elapsed))
    }
    $top = $t.pass + $t.fail + $t.skip
    $subs = $t.spass + $t.sfail + $t.sskip
    $summary = ('tag {0}: {1} tests: {2} passed, {3} failed, {4} skipped (subtests: {5} passed, {6} failed, {7} skipped); packages: {8} ok, {9} failed, {10} without tests; build failures: {11}' -f
        $Tags, $top, $t.pass, $t.fail, $t.skip, $t.spass, $t.sfail, $t.sskip, $pkgOk, $pkgFail, $pkgNoTests, $buildFailed.Count)

    $txt = [System.Collections.Generic.List[string]]::new()
    $txt.Add("go test ($($Pkgs -join ' '), tag $Tags$(if ($Shuffle) { ', shuffle on' })): exit code $GoExit")
    $txt.Add($summary)
    if ($seeds.Count) { $txt.Add("shuffle seeds: $((@($seeds | Select-Object -Unique)) -join ', ')") }
    if ($nonJson) { $txt.Add("non-JSON lines in the stream: $nonJson") }
    $txt.Add('')
    $txt.AddRange($rows)
    if ($buildFailed.Count) { $txt.Add(''); $txt.Add('BUILD FAILURES:'); foreach ($b in $buildFailed) { $txt.Add("  $b") } }
    if ($failedTests.Count) {
        $txt.Add(''); $txt.Add("FAILED TESTS ($($failedTests.Count), subtests included):")
        foreach ($f in ($failedTests | Sort-Object)) { $txt.Add("  $f") }
    }
    [System.IO.File]::WriteAllLines($TxtPath, $txt)

    Write-Host ''
    foreach ($r in $rows) { Write-Host $r }
    if ($buildFailed.Count) { Write-Host "build failures: $($buildFailed -join ', ')" }
    if ($failedTests.Count) {
        Write-Host "failed tests ($($failedTests.Count), first 40):"
        foreach ($f in ($failedTests | Sort-Object | Select-Object -First 40)) { Write-Host "  $f" }
    }
    Write-Host "counts: $TxtPath"
    return [pscustomobject]@{ Summary = $summary; Failed = ($t.fail + $t.sfail + $buildFailed.Count + $pkgFail) }
}

function Assert-WindowsForDefaultTag {
    if ($Tags -eq 'default' -and -not $IsWindows) {
        [Console]::Error.WriteLine("Stage '$Stage' with the default tag needs Windows: the clinic (local) server is Windows-only by design. Use -Tags cloud on this OS.")
        exit 1
    }
}

function Get-SortedUnique([string[]]$Items) {
    <# Ordinal sort without duplicates or empty entries. #>
    $set = [System.Collections.Generic.SortedSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($i in $Items) { if ($i) { $null = $set.Add($i) } }
    return , ([string[]]@($set))
}

function Compare-Ratchet([string[]]$Current, [string[]]$Baseline) {
    <# New = in Current only (a regression); Gone = in Baseline only (the baseline can tighten). #>
    $cur = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($c in $Current) { if ($c) { $null = $cur.Add($c) } }
    $base = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($b in $Baseline) { if ($b) { $null = $base.Add($b) } }
    $new = [System.Collections.Generic.List[string]]::new()
    foreach ($c in $cur) { if (-not $base.Contains($c)) { $new.Add($c) } }
    $gone = [System.Collections.Generic.List[string]]::new()
    foreach ($b in $base) { if (-not $cur.Contains($b)) { $gone.Add($b) } }
    [pscustomobject]@{ New = (Get-SortedUnique $new.ToArray()); Gone = (Get-SortedUnique $gone.ToArray()) }
}

function Read-Baseline([string]$Name) {
    $path = Join-Path $BaselineDir $Name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "missing scripts/baseline/$Name (the stage writes the current state to -ArtifactsDir; review it and commit it as the baseline)"
    }
    return ([System.IO.File]::ReadAllText($path) | ConvertFrom-Json)
}

function Get-BaselineList($Baseline, [string[]]$Path) {
    <# A string list inside a baseline object, or an empty list when the key is absent. #>
    $node = $Baseline
    foreach ($p in $Path) {
        if ($null -eq $node -or -not $node.PSObject.Properties[$p]) { return , ([string[]]@()) }
        $node = $node.$p
    }
    return , ([string[]]@($node | Where-Object { $_ }))
}

function Get-ToolLabel([string]$Module) {
    <# 'golang.org/x/vuln/cmd/govulncheck@v1.8.0' -> 'govulncheck v1.8.0' #>
    $path, $version = $Module -split '@', 2
    $name = @($path -split '/' | Where-Object { $_ -notmatch '^v\d+$' })[-1]
    return "$name $version"
}

function Write-JsonFile([string]$Path, $Value) {
    <# UTF-8 without BOM, LF line endings, trailing newline. #>
    $json = (ConvertTo-Json -InputObject $Value -Depth 8) -replace "`r`n", "`n"
    [System.IO.File]::WriteAllText($Path, "$json`n", [System.Text.UTF8Encoding]::new($false))
}

function Write-RatchetReport([string]$Label, $Ratchet, [string]$NewNote) {
    foreach ($n in $Ratchet.New) { Write-Host "  ${Label}: NEW $NewNote$n" }
    foreach ($g in $Ratchet.Gone) { Write-Host "  ${Label}: can tighten (no longer found): $g" }
}

function Invoke-NativeToFile([string]$FilePath, [string[]]$Arguments, [string]$OutFile, [hashtable]$ExtraEnv = @{}) {
    <# Runs a program with stdout written byte for byte to OutFile (stderr stays on the console). #>
    $psi = [System.Diagnostics.ProcessStartInfo]::new($FilePath)
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    foreach ($k in $ExtraEnv.Keys) { $psi.Environment[$k] = $ExtraEnv[$k] }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.WorkingDirectory = $RepoRoot
    Write-Host "> $(Split-Path -Leaf $FilePath) $($Arguments -join ' ')$(Format-EnvNote $ExtraEnv)   [stdout: $OutFile]"
    $p = [System.Diagnostics.Process]::Start($psi)
    $fs = [System.IO.File]::Create($OutFile)
    try { $p.StandardOutput.BaseStream.CopyTo($fs) } finally { $fs.Dispose() }
    $p.WaitForExit()
    $code = $p.ExitCode
    $p.Dispose()
    return $code
}

function Invoke-NativeCapture([string]$FilePath, [string[]]$Arguments) {
    <# Runs a program with stdout kept in memory only (stderr stays on the console). Returns @{ Code; Out }. #>
    $psi = [System.Diagnostics.ProcessStartInfo]::new($FilePath)
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.StandardOutputEncoding = [System.Text.UTF8Encoding]::new($false)
    $psi.WorkingDirectory = $RepoRoot
    Write-Host "> $(Split-Path -Leaf $FilePath) $($Arguments -join ' ')   [stdout kept in memory]"
    $p = [System.Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEnd()
    $p.WaitForExit()
    $res = @{ Code = $p.ExitCode; Out = $out }
    $p.Dispose()
    return $res
}

function Install-GoTool([string]$Module) {
    <#
      Builds a pinned tool into the scratch area (go install with GOBIN) and returns the binary path. The
      binary is built for this machine and needs no module proxy to run, so it can run inside the network
      jail and analyse another target OS than the host.
    #>
    $bin = Join-Path $NodesDir 'tools'
    New-Item -ItemType Directory -Force -Path $bin | Out-Null
    $code = Invoke-Go -Arguments @('install', $Module) -ExtraEnv @{ GOBIN = $bin; GOOS = $null; GOARCH = $null }
    if ($code -ne 0) { throw "go install $Module failed (exit $code)" }
    $name = (Get-ToolLabel $Module).Split(' ')[0]
    $exe = Join-Path $bin $(if ($IsWindows) { "$name.exe" } else { $name })
    if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "go install $Module produced no $exe" }
    return $exe
}

# Stages

function Invoke-WorkflowsStage {
    <#
      workflows: actionlint over the files in .github/workflows. go run builds the pinned version, so local
      runs and GitHub Actions check with the same rules; nothing from this repo is built or run. go run asks
      the module proxy about the module even when it is cached, so this stage is not jailed.
      A job waiting for a stage that does not exist yet is switched off with a literal "if: false"; the
      constant-condition notice for exactly that is ignored.
    #>
    $module = 'github.com/rhysd/actionlint/cmd/actionlint@v1.7.12'
    $ignore = 'constant expression .false. in condition'
    $files = @(Get-ChildItem -LiteralPath (Join-Path $RepoRoot '.github/workflows') -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Extension -in '.yml', '.yaml' } | Sort-Object Name | ForEach-Object { ".github/workflows/$($_.Name)" })
    if (-not $files.Count) {
        Write-Host 'Summary: workflows: no workflow files in .github/workflows'
        return 1
    }
    Write-Host "> go run $module -oneline -no-color -ignore '$ignore' $($files -join ' ')"
    $global:LASTEXITCODE = 99
    $out = @(& go run $module -oneline -no-color -ignore $ignore @files)
    $code = $LASTEXITCODE
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'actionlint.txt'), [string[]]$out)
    $problems = 0
    foreach ($line in $out) {
        Write-Host $line
        if ($line -notmatch '^([^:\s]+):(\d+):(\d+): (.*)$') { continue }
        $problems++
        # On GitHub Actions each problem also becomes an annotation on its workflow line.
        if ($OnActions) { Write-Host "::error title=actionlint,file=$($Matches[1]),line=$($Matches[2]),col=$($Matches[3])::$($Matches[4] -replace '%', '%25')" }
    }
    $state = if ($code -eq 0) { 'clean' } elseif ($problems) { "$problems problems" } else { "actionlint failed (exit $code)" }
    Write-Host "Summary: workflows: $($module.Split('/')[-1] -replace '@', ' '), $($files.Count) files, $state"
    return $(if ($code -eq 0) { 0 } else { 1 })
}

function Read-CommittedEnv([string]$Rel) {
    <# KEY -> value of a committed env file at HEAD ($null when it is not committed). Values stay in memory. #>
    $global:LASTEXITCODE = 99
    $lines = @(& git -C $RepoRoot show "HEAD:$Rel" 2>$null)
    if ($LASTEXITCODE -ne 0) { return $null }
    $values = [ordered]@{}
    foreach ($line in $lines) {
        if ($line -notmatch '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') { continue }
        $key = $Matches[1]
        $v = $Matches[2].Trim()
        if ($v.Length -ge 2 -and (($v[0] -eq '"' -and $v[-1] -eq '"') -or ($v[0] -eq "'" -and $v[-1] -eq "'"))) { $v = $v.Substring(1, $v.Length - 2) }
        $values[$key] = $v
    }
    return $values
}

function Invoke-EnvCheckStage {
    $problems = [System.Collections.Generic.List[string]]::new()
    $parts = [System.Collections.Generic.List[string]]::new()
    $tracked = @(& git -C $RepoRoot ls-files -- 'internal/config/*.env' | Where-Object { $_ })
    foreach ($t in $tracked) { $problems.Add("${t}: tracked (override files stay out of git)") }
    $shared = @{ SYNC_SECRET = [System.Collections.Generic.HashSet[string]]::new(); DB_ENCRYPTION_KEY = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase) }
    foreach ($rel in $ConfigFiles.Values) {
        $name = Split-Path -Leaf $rel
        $before = $problems.Count
        $defaults = Read-CommittedEnv "$rel.defaults"
        if ($null -eq $defaults) { $problems.Add("$name.defaults: not committed"); $parts.Add("$name FAIL"); continue }
        $secrets = [System.Collections.Generic.List[string]]::new()
        foreach ($key in $defaults.Keys) {
            $v = $defaults[$key]
            if ($key -match 'SECRET$') {
                $secrets.Add($key)
                if (-not $v.StartsWith('dev-only-')) { $problems.Add("$name.defaults: $key is not a dev-only- value") }
            }
            if ($key -eq 'DB_ENCRYPTION_KEY') {
                $secrets.Add($key)
                if ($v -notmatch '^[0-9A-Fa-f]{64}$') { $problems.Add("$name.defaults: $key is not 64 hex characters") }
            }
            if ($key -in 'PEER_URL', 'PUBLIC_URL' -and $v) { $problems.Add("$name.defaults: $key is not empty") }
            if ($shared.ContainsKey($key)) { $null = $shared[$key].Add($v) }
        }
        foreach ($k in $shared.Keys) { if (-not $defaults.Contains($k)) { $problems.Add("$name.defaults: no $k") } }
        $example = Read-CommittedEnv "$rel.example"
        if ($null -eq $example) {
            $problems.Add("$name.example: not committed")
        } else {
            $missing = @($defaults.Keys | Where-Object { -not $example.Contains($_) })
            $extra = @($example.Keys | Where-Object { -not $defaults.Contains($_) })
            $filled = @($example.Keys | Where-Object { $example[$_] })
            if ($missing.Count) { $problems.Add("$name.example: lacks $($missing -join ', ')") }
            if ($extra.Count) { $problems.Add("$name.example: has keys the defaults lack: $($extra -join ', ')") }
            if ($filled.Count) { $problems.Add("$name.example: values for $($filled -join ', ')") }
        }
        $global:LASTEXITCODE = 99
        $null = & git -C $RepoRoot check-ignore -q --no-index -- $rel 2>$null
        if ($LASTEXITCODE -ne 0) { $problems.Add("${rel}: not ignored by git") }
        $defaults = $null; $example = $null
        Write-Host "  ${name}: defaults $(if ($problems.Count -gt $before) { 'FAIL' } else { 'dev values only' }) ($($secrets -join ', '))"
        $parts.Add("$name $(if ($problems.Count -gt $before) { 'FAIL' } else { 'ok' })")
    }
    foreach ($k in $shared.Keys) {
        if ($shared[$k].Count -gt 1) { $problems.Add("$k differs between local.env.defaults and cloud.env.defaults") }
    }
    $shared = $null
    foreach ($p in $problems) { Write-Host "  PROBLEM: $p" }
    Write-Host "Summary: $($parts -join '; '); tracked env files: $($tracked.Count)$(if ($problems.Count) { "; $($problems.Count) problems" })"
    return $(if ($problems.Count) { 1 } else { 0 })
}

function Invoke-PlatformStage {
    Initialize-GoModules
    $null = Initialize-ClientDist
    $fail = $false
    $notes = [System.Collections.Generic.List[string]]::new()
    Enter-GoJail
    try {
        $checks = @(@{ Label = 'cloud'; Env = @{} })
        if (-not $IsLinux) { $checks += @{ Label = 'cloud GOOS=linux'; Env = @{ GOOS = 'linux'; GOARCH = 'amd64'; CGO_ENABLED = '0' } } }
        foreach ($c in $checks) {
            $deps = Get-GoLines -Arguments @('list', '-deps', '-tags', 'cloud', './cmd/server') -ExtraEnv $c.Env
            if ($script:GoExit -ne 0) { Write-Host "FAIL ($($c.Label)): go list failed"; $fail = $true; $notes.Add("$($c.Label): go list failed"); continue }
            $tray = @($deps | Where-Object { $_ -match 'systray' -and $_ -notmatch '/internal/systray$' })
            if ($tray.Count) {
                Write-Host "FAIL ($($c.Label)): the cloud build depends on $($tray -join ', ')"
                $fail = $true; $notes.Add("$($c.Label): tray library present")
            } else {
                Write-Host "ok ($($c.Label)): no tray library among $($deps.Count) packages"
                $notes.Add("$($c.Label): no tray library")
            }
        }
        if ($IsWindows) {
            $deps = Get-GoLines -Arguments @('list', '-deps', './cmd/server')
            $has = @($deps | Where-Object { $_ -match '^github\.com/getlantern/systray' }).Count -gt 0
            Write-Host "info (default tag): tray library $(if ($has) { 'present' } else { 'absent' }) in the clinic build"
        } else {
            Write-Host 'info: the clinic (local) server is Windows-only by design; default-tag stages refuse to run on this OS'
        }
    } finally {
        Exit-GoJail
    }
    Write-Host "Summary: $($notes -join '; ')"
    return $(if ($fail) { 1 } else { 0 })
}

function Invoke-VetStage {
    $runs = @()
    if ($Tags -eq 'cloud') {
        $runs += @{ Label = 'cloud'; Args = @('vet', '-tags', 'cloud') + $Pkgs; Env = @{} }
        if (-not $IsLinux) { $runs += @{ Label = 'cloud GOOS=linux'; Args = @('vet', '-tags', 'cloud') + $Pkgs; Env = @{ GOOS = 'linux'; GOARCH = 'amd64'; CGO_ENABLED = '0' } } }
    } elseif ($IsWindows) {
        $runs += @{ Label = 'default'; Args = @('vet') + $Pkgs; Env = @{} }
    } else {
        # The clinic server is Windows-only; off Windows it is only type-checked for windows.
        $runs += @{ Label = 'default GOOS=windows'; Args = @('vet') + $Pkgs; Env = @{ GOOS = 'windows'; GOARCH = 'amd64' } }
    }
    Initialize-GoModules
    $null = Initialize-ClientDist
    $results = [System.Collections.Generic.List[string]]::new()
    $fail = $false
    Write-TestConfig -PortOverrides $PortOverrides -Peer $PeerUrl
    try {
        Enter-GoJail
        foreach ($r in $runs) {
            $code = Invoke-Go -Arguments $r.Args -ExtraEnv $r.Env
            if ($code -ne 0) { $fail = $true; $results.Add("$($r.Label) FAIL") } else { $results.Add("$($r.Label) clean") }
        }
    } finally {
        Exit-GoJail
        if (-not (Restore-CommittedConfig)) { $fail = $true; $results.Add('config restore FAILED') }
    }
    Write-Host "Summary: vet $($results -join '; ')"
    return $(if ($fail) { 1 } else { 0 })
}

function Invoke-GoTestStage {
    Assert-WindowsForDefaultTag
    Initialize-GoModules
    $null = Initialize-ClientDist
    $json = Join-Path $ArtifactsDir "go-test-$Tags.json"
    $txt = Join-Path $ArtifactsDir "go-test-$Tags.txt"
    $goExit = 1
    $restored = $false
    Write-TestConfig -PortOverrides $PortOverrides -Peer $PeerUrl
    try {
        Enter-GoJail
        $goArgs = @('test')
        if ($Tags -eq 'cloud') { $goArgs += @('-tags', 'cloud') }
        $goArgs += @('-count=1', '-json')
        if ($Shuffle) { $goArgs += '-shuffle=on' }
        $goArgs += $Pkgs
        $goExit = Invoke-GoToFile -Arguments $goArgs -OutFile $json
    } finally {
        Exit-GoJail
        $restored = Restore-CommittedConfig
        $report = Write-GoTestReport -JsonPath $json -TxtPath $txt -GoExit $goExit
    }
    Write-Host "Summary: $($report.Summary)$(if (-not $restored) { '; config restore FAILED' })"
    return $(if ($goExit -ne 0 -or $report.Failed -gt 0 -or -not $restored) { 1 } else { 0 })
}

function Invoke-ContractStage {
    Assert-WindowsForDefaultTag
    $dash = ''
    if ($DashboardDir) {
        $dash = [System.IO.Path]::GetFullPath($DashboardDir)
        if (-not (Test-Path -LiteralPath (Join-Path $dash 'src/lib/api.ts') -PathType Leaf)) {
            throw "-DashboardDir $dash is not a dashboard checkout (no src/lib/api.ts)"
        }
        Write-Host "contract: dashboard source for the parity tests: $dash"
    } else {
        Write-Host 'contract: no -DashboardDir given; the parity tests will skip'
    }
    Initialize-GoModules
    $null = Initialize-ClientDist
    $script:Pkgs = @('./internal/api/server')
    $json = Join-Path $ArtifactsDir "contract-$Tags.json"
    $txt = Join-Path $ArtifactsDir "contract-$Tags.txt"
    $goExit = 1
    $restored = $false
    $savedDash = $null
    Write-TestConfig -PortOverrides $PortOverrides -Peer $PeerUrl
    try {
        Enter-GoJail
        $savedDash = Set-TempEnv @{ DASHBOARD_DIR = $(if ($dash) { $dash } else { $null }) }
        $goArgs = @('test')
        if ($Tags -eq 'cloud') { $goArgs += @('-tags', 'cloud') }
        $goArgs += @('-count=1', '-json', '-run', '^(TestContract|TestParity)') + $script:Pkgs
        $goExit = Invoke-GoToFile -Arguments $goArgs -OutFile $json
    } finally {
        if ($null -ne $savedDash) { $null = Set-TempEnv $savedDash }
        Exit-GoJail
        $restored = Restore-CommittedConfig
        $report = Write-GoTestReport -JsonPath $json -TxtPath $txt -GoExit $goExit
    }
    Write-Host "Summary: contract $($report.Summary)$(if (-not $dash) { '; parity skipped (no -DashboardDir)' })$(if (-not $restored) { '; config restore FAILED' })"
    return $(if ($goExit -ne 0 -or $report.Failed -gt 0 -or -not $restored) { 1 } else { 0 })
}

function Invoke-FmtStage {
    # The gofmt of the toolchain go.mod selects, so the result does not depend on the installed Go.
    $goroot = (& go env GOROOT | Out-String).Trim()
    $gofmt = Join-Path $goroot (Join-Path 'bin' $(if ($IsWindows) { 'gofmt.exe' } else { 'gofmt' }))
    if (-not (Test-Path -LiteralPath $gofmt -PathType Leaf)) { throw "gofmt not found at $gofmt" }
    $files = @((& git -C $RepoRoot ls-files -z -- '*.go' ':(exclude)client/dist/**' | Out-String) -split "`0" |
        ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if (-not $files.Count) { throw 'git ls-files found no Go files' }
    Write-Host "> gofmt -l ($($files.Count) tracked Go files, client/dist excluded)"
    $flagged = [System.Collections.Generic.List[string]]::new()
    $errors = $false
    for ($i = 0; $i -lt $files.Count; $i += 100) {
        $batch = $files[$i..([Math]::Min($i + 99, $files.Count - 1))]
        $global:LASTEXITCODE = 99
        $out = @(& $gofmt -l -- @batch)
        if ($LASTEXITCODE -ne 0) { $errors = $true }
        foreach ($f in $out) { if ($f.Trim()) { $flagged.Add(($f.Trim() -replace '\\', '/')) } }
    }
    # A CRLF checkout flags every file; check those again with LF endings.
    $tmp = Join-Path $NodesDir 'tmp'
    $unformatted = [System.Collections.Generic.List[string]]::new()
    foreach ($f in $flagged) {
        $text = [System.IO.File]::ReadAllText((Join-Path $RepoRoot $f))
        if (-not $text.Contains("`r")) { $unformatted.Add($f); continue }
        New-Item -ItemType Directory -Force -Path $tmp | Out-Null
        $lf = Join-Path $tmp 'gofmt-lf.go'
        [System.IO.File]::WriteAllText($lf, $text.Replace("`r`n", "`n"), [System.Text.UTF8Encoding]::new($false))
        $again = @(& $gofmt -l -- $lf)
        Remove-Item -LiteralPath $lf -Force
        if ($again.Count) { $unformatted.Add($f) }
    }
    $current = Get-SortedUnique $unformatted.ToArray()
    $baseline = Get-BaselineList (Read-Baseline 'gofmt.json') @('files')
    $r = Compare-Ratchet $current $baseline
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'gofmt.txt'), [string[]]$current)
    Write-JsonFile (Join-Path $ArtifactsDir 'baseline-gofmt.json') ([ordered]@{ files = $current })
    Write-Host "unformatted: $($current.Count) (baseline $($baseline.Count))"
    foreach ($f in $current) { Write-Host "  $f$(if ($r.New -contains $f) { '   NEW' })" }
    Write-RatchetReport -Label 'gofmt' -Ratchet $r -NewNote 'unformatted file: '
    if ($errors) { Write-Host 'gofmt reported errors (see above)' }
    Write-Host ("Summary: fmt: {0} files, {1} unformatted, {2} new{3}{4}" -f $files.Count, $current.Count, $r.New.Count,
        $(if ($r.Gone.Count) { "; can tighten: $($r.Gone.Count)" } else { '' }), $(if ($errors) { '; gofmt errors' } else { '' }))
    return $(if ($errors -or $r.New.Count) { 1 } else { 0 })
}

function Invoke-ModStage {
    # No code is built or run here. tidy resolves the test imports of the dependencies as well, which a
    # plain download does not fetch, so this stage uses the module proxy instead of the network jail.
    Initialize-GoModules
    $lines = [System.Collections.Generic.List[string]]::new()
    $notes = [System.Collections.Generic.List[string]]::new()
    $fail = $false
    $verify = Get-GoLines -Arguments @('mod', 'verify')
    $lines.Add("go mod verify: exit $script:GoExit"); $lines.AddRange([string[]]$verify)
    if ($script:GoExit -ne 0) { $fail = $true; $notes.Add('verify FAILED') } else { $notes.Add('verify clean') }
    $diff = Get-GoLines -Arguments @('mod', 'tidy', '-diff')
    $lines.Add("go mod tidy -diff: exit $script:GoExit"); $lines.AddRange([string[]]$diff)
    if ($script:GoExit -ne 0 -or $diff.Count) { $fail = $true; $notes.Add('tidy -diff NOT clean') } else { $notes.Add('tidy -diff clean') }
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'mod.txt'), $lines)
    foreach ($l in $lines) { Write-Host "  $l" }
    Write-Host "Summary: mod: $($notes -join '; ')"
    return $(if ($fail) { 1 } else { 0 })
}

function Read-GovulncheckStream([string]$Path) {
    <# Findings of a govulncheck -format json stream: per OSV id the most precise level reached. #>
    $rank = @{ module = 1; package = 2; symbol = 3 }
    $found = @{}
    $config = @{}
    $reader = [System.IO.StreamReader]::new($Path)
    $json = [Newtonsoft.Json.JsonTextReader]::new($reader)
    $json.SupportMultipleContent = $true
    $json.DateParseHandling = [Newtonsoft.Json.DateParseHandling]::None
    try {
        while ($json.Read()) {
            $msg = [Newtonsoft.Json.Linq.JToken]::ReadFrom($json)
            if ($msg -isnot [Newtonsoft.Json.Linq.JObject]) { continue }
            $cfg = $msg['config']
            if ($null -ne $cfg) {
                foreach ($k in 'scanner_version', 'db_last_modified', 'go_version', 'scan_level') { $config[$k] = [string]$cfg[$k] }
            }
            $finding = $msg['finding']
            if ($null -eq $finding) { continue }
            $id = [string]$finding['osv']
            # JTokens are enumerable: assign them directly, never through a pipeline or an if-expression.
            $trace = $finding['trace']
            $frame = $null
            if ($null -ne $trace -and $trace.Count -gt 0) { $frame = $trace[0] }
            $level = 'module'
            if ($null -ne $frame -and [string]$frame['function']) { $level = 'symbol' }
            elseif ($null -ne $frame -and [string]$frame['package']) { $level = 'package' }
            if (-not $found.ContainsKey($id) -or $rank[$level] -gt $rank[$found[$id].Level]) {
                $found[$id] = [pscustomobject]@{
                    Level  = $level
                    Module = $(if ($null -ne $frame) { "$([string]$frame['module'])@$([string]$frame['version'])" } else { '' })
                    Fixed  = [string]$finding['fixed_version']
                }
            }
        }
    } finally {
        $json.Close()
        $reader.Dispose()
    }
    [pscustomobject]@{ Config = $config; Found = $found }
}

function Invoke-VulnStage {
    Initialize-GoModules
    $null = Initialize-ClientDist
    $tool = Install-GoTool $GovulncheckModule
    $baseline = Read-Baseline 'govulncheck.json'
    $tagList = if ($TagsGiven) { @($Tags) } else { @('default', 'cloud') }
    $current = [ordered]@{}
    $txt = [System.Collections.Generic.List[string]]::new()
    $notes = [System.Collections.Generic.List[string]]::new()
    $fail = $false
    foreach ($tag in $tagList) {
        # Each build is analysed for the OS it ships on.
        $envs = if ($tag -eq 'cloud') { @{ GOOS = 'linux'; GOARCH = 'amd64'; CGO_ENABLED = '0' } } else { @{ GOOS = 'windows'; GOARCH = 'amd64' } }
        $out = Join-Path $ArtifactsDir "govulncheck-$tag.json"
        $toolArgs = @('-format', 'json')
        if ($tag -eq 'cloud') { $toolArgs += @('-tags', 'cloud') }
        $code = Invoke-NativeToFile -FilePath $tool -Arguments ($toolArgs + './...') -OutFile $out -ExtraEnv $envs
        if ($code -ne 0) {
            $fail = $true; $notes.Add("${tag}: govulncheck exit $code")
            Write-Host "govulncheck ($tag) failed with exit code $code"
            continue
        }
        $scan = Read-GovulncheckStream $out
        $ids = @($scan.Found.Keys)
        $reachable = Get-SortedUnique @($ids | Where-Object { $scan.Found[$_].Level -eq 'symbol' })
        $imported = @($ids | Where-Object { $scan.Found[$_].Level -eq 'package' }).Count
        $required = @($ids | Where-Object { $scan.Found[$_].Level -eq 'module' }).Count
        $current[$tag] = $reachable
        $r = Compare-Ratchet $reachable (Get-BaselineList $baseline @('reachable', $tag))
        $c = $scan.Config
        $txt.Add("tag ${tag}: $($reachable.Count) reachable, $imported imported but not called, $required required only (govulncheck $($c['scanner_version']), $($c['go_version']), database $($c['db_last_modified']))")
        foreach ($id in $reachable) {
            $f = $scan.Found[$id]
            $txt.Add("  $id  $($f.Module)$(if ($f.Fixed) { "  fixed in $($f.Fixed)" })$(if ($r.New -contains $id) { '  NEW' })")
        }
        Write-RatchetReport -Label "vuln $tag" -Ratchet $r -NewNote 'reachable: '
        if ($r.New.Count) { $fail = $true }
        $notes.Add("${tag}: $($reachable.Count) reachable ($($r.New.Count) new$(if ($r.Gone.Count) { ", can tighten: $($r.Gone.Count)" }))")
    }
    foreach ($t in $txt) { Write-Host $t }
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'vuln.txt'), $txt)
    $state = [ordered]@{ tool = (Get-ToolLabel $GovulncheckModule); reachable = [ordered]@{} }
    foreach ($tag in @('cloud', 'default')) {
        $ids = Get-BaselineList $baseline @('reachable', $tag)
        if ($current.Contains($tag)) { $ids = $current[$tag] }
        $state.reachable[$tag] = $ids
    }
    Write-JsonFile (Join-Path $ArtifactsDir 'baseline-govulncheck.json') $state
    Write-Host "Summary: vuln: $($notes -join '; ')"
    return $(if ($fail) { 1 } else { 0 })
}

function Export-CommittedTree {
    <# The files of HEAD, exported into the scratch area, so a scan sees only committed content. #>
    $dir = Join-Path $NodesDir 'committed-tree'
    if (Test-Path -LiteralPath $dir) { Remove-Item -LiteralPath $dir -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $NodesDir | Out-Null
    $zip = Join-Path $NodesDir 'committed-tree.zip'
    $global:LASTEXITCODE = 99
    & git -C $RepoRoot archive --format=zip -o $zip HEAD
    if ($LASTEXITCODE -ne 0) { throw "git archive failed (exit $LASTEXITCODE)" }
    [System.IO.Compression.ZipFile]::ExtractToDirectory($zip, $dir)
    Remove-Item -LiteralPath $zip -Force
    return $dir
}

function Invoke-GitleaksScan([string]$Tool, [string[]]$Target, [string]$Config, [string]$StripPrefix = '') {
    <#
      Runs one gitleaks scan with --redact and the JSON report on stdout, held in memory only. Returns
      rule, file, line and commit per finding; the matched text and author fields are dropped here.
    #>
    $res = Invoke-NativeCapture -FilePath $Tool -Arguments ($Target + @('--config', $Config, '--redact',
            '--no-banner', '--no-color', '--log-level', 'warn', '--exit-code', '0', '--report-format', 'json', '--report-path', '-'))
    if ($res.Code -ne 0) { throw "gitleaks $($Target[0]) failed (exit $($res.Code))" }
    $raw = @($res.Out | ConvertFrom-Json)
    $res = $null
    $prefix = ($StripPrefix -replace '\\', '/').TrimEnd('/') + '/'
    $findings = [System.Collections.Generic.List[object]]::new()
    foreach ($f in $raw) {
        if ($null -eq $f) { continue }
        $file = ([string]$f.File) -replace '\\', '/'
        if ($StripPrefix -and $file.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) { $file = $file.Substring($prefix.Length) }
        $findings.Add([pscustomobject]@{ rule = [string]$f.RuleID; file = $file; line = [int]$f.StartLine; commit = [string]$f.Commit })
    }
    $raw = $null
    return , $findings.ToArray()
}

function Invoke-SecretsStage {
    $config = Join-Path $RepoRoot '.gitleaks.toml'
    if (-not (Test-Path -LiteralPath $config -PathType Leaf)) { throw 'missing .gitleaks.toml at the repo root' }
    $baseline = Read-Baseline 'gitleaks.json'
    # Build the scanner with network access; the scans run inside the jail.
    $tool = Install-GoTool $GitleaksModule
    $tree = Export-CommittedTree
    Enter-GoJail
    try {
        $dirFindings = Invoke-GitleaksScan -Tool $tool -Target @('dir', $tree) -Config $config -StripPrefix $tree
        $gitFindings = Invoke-GitleaksScan -Tool $tool -Target @('git', $RepoRoot) -Config $config
    } finally {
        Exit-GoJail
        if (Test-Path -LiteralPath $tree) { Remove-Item -LiteralPath $tree -Recurse -Force }
    }
    $dirPrints = Get-SortedUnique @($dirFindings | ForEach-Object { "$($_.file):$($_.rule):$($_.line)" })
    $gitPrints = Get-SortedUnique @($gitFindings | ForEach-Object { "$($_.commit):$($_.file):$($_.rule):$($_.line)" })
    $d = Compare-Ratchet $dirPrints (Get-BaselineList $baseline @('dir'))
    $g = Compare-Ratchet $gitPrints (Get-BaselineList $baseline @('git'))

    Write-JsonFile (Join-Path $ArtifactsDir 'gitleaks-dir.json') @($dirFindings | Select-Object rule, file, line)
    Write-JsonFile (Join-Path $ArtifactsDir 'gitleaks-git.json') @($gitFindings | Select-Object commit, rule, file, line)
    Write-JsonFile (Join-Path $ArtifactsDir 'baseline-gitleaks.json') ([ordered]@{
            tool = (Get-ToolLabel $GitleaksModule); dir = $dirPrints; git = $gitPrints })

    Write-Host "committed tree: $($dirPrints.Count) findings (baseline $((Get-BaselineList $baseline @('dir')).Count))"
    foreach ($p in $dirPrints) { Write-Host "  $p$(if ($d.New -contains $p) { '   NEW' })" }
    Write-RatchetReport -Label 'tree' -Ratchet $d -NewNote 'finding: '
    Write-Host "history (report only): $($gitPrints.Count) findings, $($g.New.Count) not in the baseline"
    Write-RatchetReport -Label 'history' -Ratchet $g -NewNote '(report only) finding: '
    Write-Host ("Summary: secrets: tree {0} findings, {1} new{2}; history {3} findings, {4} new (report only){5}" -f
        $dirPrints.Count, $d.New.Count, $(if ($d.Gone.Count) { ", can tighten: $($d.Gone.Count)" } else { '' }),
        $gitPrints.Count, $g.New.Count, $(if ($g.Gone.Count) { ", can tighten: $($g.Gone.Count)" } else { '' }))
    return $(if ($d.New.Count) { 1 } else { 0 })
}

# System and packaging stages

$SystemPorts = [ordered]@{ proxy = 55580; clinic = 55581; cloud = 55582; altCloud = 55555 }
$AltCloudVersion = '0.0.0-systest'

function Invoke-ChildScript([string]$Script, [string[]]$Arguments, [hashtable]$ExtraEnv = @{}) {
    <# Runs a script of this folder in a new pwsh and returns its last JSON output line. #>
    $saved = Set-TempEnv $ExtraEnv
    $lines = [System.Collections.Generic.List[string]]::new()
    try {
        Write-Host "> $(Split-Path -Leaf $Script) $($Arguments -join ' ')"
        $global:LASTEXITCODE = 99
        & ([Environment]::ProcessPath) -NoProfile -NonInteractive -File $Script @Arguments | ForEach-Object {
            $lines.Add("$_")
            Write-Host "  $_"
        }
        $code = $LASTEXITCODE
    } finally {
        $null = Set-TempEnv $saved
    }
    if ($code -ne 0) { throw "$(Split-Path -Leaf $Script) failed (exit $code)" }
    $json = @($lines | Where-Object { $_.StartsWith('{') })
    if (-not $json.Count) { throw "$(Split-Path -Leaf $Script) printed no result line" }
    return ($json[-1] | ConvertFrom-Json)
}

function Get-SystemSteps([string]$JsonPath) {
    <# The TestSystem steps of a go test -json stream: result, seconds and the test's own log lines. #>
    Set-StrictMode -Off
    $steps = [ordered]@{}
    if (-not (Test-Path -LiteralPath $JsonPath)) { return $steps }
    foreach ($line in [System.IO.File]::ReadLines($JsonPath)) {
        if (-not $line.StartsWith('{') -or -not $line.Contains('"TestSystem/')) { continue }
        $ev = $line | ConvertFrom-Json
        if ($ev.Test -notmatch '^TestSystem/([^/]+)$') { continue }
        $name = $Matches[1]
        if (-not $steps.Contains($name)) { $steps[$name] = [pscustomobject]@{ Name = $name; Result = 'incomplete'; Seconds = 0.0; Lines = [System.Collections.Generic.List[string]]::new() } }
        $s = $steps[$name]
        if ($ev.Action -in 'pass', 'fail', 'skip') {
            $s.Result = $ev.Action
            if ($ev.Elapsed) { $s.Seconds = [double]$ev.Elapsed }
        } elseif ($ev.Action -eq 'output' -and $ev.Output -match '^\s+\S+_test\.go:\d+: ') {
            $s.Lines.Add($ev.Output.TrimEnd())
        }
    }
    return $steps
}

function Invoke-SystemStage {
    Assert-WindowsForDefaultTag
    Initialize-GoModules
    $null = Initialize-ClientDist
    $stack = Join-Path $PSScriptRoot 'stack.ps1'
    $bin = if ($BinDir) { [System.IO.Path]::GetFullPath($BinDir) } else { Join-Path $NodesDir 'bin' }
    New-Item -ItemType Directory -Force -Path $bin | Out-Null
    $work = Join-Path $NodesDir ('systest-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
    if (Test-Path -LiteralPath $work) { throw "the work folder exists already: $work" }

    # One random sync secret and database key for all three builds, so the nodes accept each other and a
    # cloud restore moves a database the clinic can open.
    $sync = New-RandomToken 48
    $publish = New-RandomToken 48
    $dbKey = New-RandomHexKey
    if ($OnActions) { Write-Host "::add-mask::$sync"; Write-Host "::add-mask::$publish"; Write-Host "::add-mask::$dbKey" }
    $shared = @{ STACK_SYNC_SECRET = $sync; STACK_PUBLISH_SECRET = $publish; STACK_DB_ENCRYPTION_KEY = $dbKey }
    $common = @('-Build', '-WorkDir', $NodesDir, '-BinDir', $bin)
    $clinic = Invoke-ChildScript $stack ($common + @('-Tags', 'default', '-Port', "$($SystemPorts.clinic)",
            '-PeerUrl', "http://127.0.0.1:$($SystemPorts.proxy)", '-Name', 'ZealClinicSystest')) $shared
    $cloud = Invoke-ChildScript $stack ($common + @('-Tags', 'cloud', '-Port', "$($SystemPorts.cloud)", '-Name', 'ZealClinicCloudSystest')) $shared
    $alt = Invoke-ChildScript $stack ($common + @('-Tags', 'cloud', '-Port', "$($SystemPorts.altCloud)",
            '-Version', $AltCloudVersion, '-Name', 'ZealClinicCloudSystestAlt')) $shared

    $Pkgs = @('./systest/...')
    $json = Join-Path $ArtifactsDir 'go-test-systest.json'
    $txt = Join-Path $ArtifactsDir 'go-test-systest.txt'
    $testEnv = @{
        SYSTEST_CLINIC_BIN = $clinic.binary; SYSTEST_CLOUD_BIN = $cloud.binary; SYSTEST_ALT_CLOUD_BIN = $alt.binary
        SYSTEST_ALT_VERSION = $AltCloudVersion; SYSTEST_WORK_DIR = $work
        SYSTEST_SYNC_SECRET = $sync; SYSTEST_PUBLISH_SECRET = $publish
        SYSTEST_PROXY_PORT = "$($SystemPorts.proxy)"; SYSTEST_CLINIC_PORT = "$($SystemPorts.clinic)"
        SYSTEST_CLOUD_PORT = "$($SystemPorts.cloud)"; SYSTEST_ALT_CLOUD_PORT = "$($SystemPorts.altCloud)"
        SYSTEST_FULL = $(if ($Full) { '1' } else { $null })
    }
    $goExit = 1
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    Enter-GoJail
    $saved = Set-TempEnv $testEnv
    try {
        $goExit = Invoke-GoToFile -Arguments @('test', '-tags', 'systest', '-count=1', '-json', '-v', '-timeout', '30m', './systest/...') -OutFile $json
    } finally {
        $null = Set-TempEnv $saved
        Exit-GoJail
        $watch.Stop()
    }
    $report = Write-GoTestReport -JsonPath $json -TxtPath $txt -GoExit $goExit

    # Node logs: console output per start, the seed run and the servers' own log files.
    $nodesOut = Join-Path $ArtifactsDir 'nodes'
    New-Item -ItemType Directory -Force -Path $nodesOut | Out-Null
    if (Test-Path -LiteralPath $work) {
        Get-ChildItem -LiteralPath $work -Recurse -File | Where-Object { $_.Name -like '*.log' -or $_.Name -like 'clinic.log*' } | ForEach-Object {
            $rel = [System.IO.Path]::GetRelativePath($work, $_.FullName) -replace '[\\/]', '-'
            Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $nodesOut $rel) -Force
        }
    }

    $steps = Get-SystemSteps $json
    $table = [System.Collections.Generic.List[string]]::new()
    $table.Add("system: binaries $(Split-Path -Leaf $clinic.binary), $(Split-Path -Leaf $cloud.binary), $(Split-Path -Leaf $alt.binary) (version $AltCloudVersion) in $bin")
    $table.Add("work folder $work; node logs in $nodesOut")
    $table.Add('')
    $table.Add(('{0,-34} {1,-10} {2,8}' -f 'STEP', 'RESULT', 'SECONDS'))
    foreach ($s in $steps.Values) {
        $table.Add(('{0,-34} {1,-10} {2,8:0.0}' -f $s.Name, $s.Result, $s.Seconds))
        foreach ($l in $s.Lines) { $table.Add("    $($l.Trim())") }
    }
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'system.txt'), $table)
    foreach ($l in $table) { Write-Host $l }

    $all = @($steps.Values)
    $passed = @($all | Where-Object Result -eq 'pass')
    $failed = @($all | Where-Object Result -eq 'fail')
    $skipped = @($all | Where-Object Result -eq 'skip')
    $timing = ($passed | ForEach-Object { '{0} {1:0.0}s' -f $_.Name, $_.Seconds }) -join ', '
    $extra = if ($failed.Count) { "; FAILED: $(($failed | ForEach-Object Name) -join ', ')" } elseif ($skipped.Count) { "; skipped: $(($skipped | ForEach-Object Name) -join ', ')" } else { '' }
    Write-Host ("Summary: system: {0} of {1} steps passed in {2:0}s{3} ({4}); {5}" -f $passed.Count, $all.Count, $watch.Elapsed.TotalSeconds, $extra, $timing, $report.Summary)
    return $(if ($goExit -ne 0 -or $report.Failed -gt 0 -or $failed.Count -or -not $all.Count) { 1 } else { 0 })
}

function Invoke-JailedProgram([string]$FilePath, [string[]]$Arguments, [string]$WorkingDirectory, [string]$LocalAppData,
    [string]$Temp, [string]$LogPath, [int]$TimeoutSec) {
    <# Runs a built program with the network jail and its own LOCALAPPDATA and TEMP; output to LogPath. #>
    $psi = [System.Diagnostics.ProcessStartInfo]::new($FilePath)
    foreach ($a in $Arguments) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.WorkingDirectory = $WorkingDirectory
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach ($k in 'NO_PROXY', 'no_proxy', 'GOFLAGS', 'GOPROXY') { $null = $psi.Environment.Remove($k) }
    $psi.Environment['HTTP_PROXY'] = $JailProxy
    $psi.Environment['HTTPS_PROXY'] = $JailProxy
    $psi.Environment['LOCALAPPDATA'] = $LocalAppData
    $psi.Environment['TEMP'] = $Temp
    $psi.Environment['TMP'] = $Temp
    Write-Host "> $(Split-Path -Leaf $FilePath) $($Arguments -join ' ')   (in $WorkingDirectory, LOCALAPPDATA $LocalAppData)"
    $p = [System.Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEndAsync()
    $err = $p.StandardError.ReadToEndAsync()
    $timedOut = -not $p.WaitForExit($TimeoutSec * 1000)
    if ($timedOut) { try { $p.Kill($true) } catch { } }
    $p.WaitForExit()
    [System.IO.File]::WriteAllText($LogPath, $out.Result + $err.Result)
    $res = [pscustomobject]@{ ExitCode = $p.ExitCode; TimedOut = $timedOut }
    $p.Dispose()
    return $res
}

function Get-PeInfo([string]$Path) {
    $fs = [System.IO.File]::OpenRead($Path)
    try {
        $br = [System.IO.BinaryReader]::new($fs)
        if ($br.ReadUInt16() -ne 0x5A4D) { return $null }
        $fs.Position = 0x3C
        $pe = $br.ReadInt32()
        $fs.Position = $pe
        if ($br.ReadUInt32() -ne 0x00004550) { return $null }
        $machine = $br.ReadUInt16()
        $sections = $br.ReadUInt16()
        $null = $br.ReadUInt32()
        $null = $br.ReadUInt32()
        $symbols = $br.ReadUInt32()
        $optionalSize = $br.ReadUInt16()
        $fs.Position = $pe + 24 + 68
        $subsystem = $br.ReadUInt16()
        # -s leaves the COFF symbol table empty (Go keeps its header), -w drops the DWARF sections (named
        # .debug_* or /<n> for long names).
        $debug = $false
        for ($i = 0; $i -lt $sections; $i++) {
            $fs.Position = $pe + 24 + $optionalSize + 40 * $i
            $name = [System.Text.Encoding]::ASCII.GetString($br.ReadBytes(8)).TrimEnd([char]0)
            if ($name.StartsWith('.debug') -or $name.StartsWith('.zdebug') -or $name.StartsWith('/')) { $debug = $true }
        }
        [pscustomobject]@{ Amd64 = ($machine -eq 0x8664); Subsystem = $(switch ($subsystem) { 2 { 'gui' } 3 { 'console' } default { "$subsystem" } })
            Stripped = ($symbols -eq 0 -and -not $debug) }
    } finally {
        $fs.Dispose()
    }
}

function Get-ElfInfo([string]$Path) {
    $fs = [System.IO.File]::OpenRead($Path)
    try {
        $br = [System.IO.BinaryReader]::new($fs)
        $ident = $br.ReadBytes(16)
        if ($ident.Length -lt 16 -or $ident[0] -ne 0x7F -or $ident[1] -ne 0x45 -or $ident[2] -ne 0x4C -or $ident[3] -ne 0x46) { return $null }
        $type = $br.ReadUInt16()
        $machine = $br.ReadUInt16()
        $fs.Position = 32
        $phoff = $br.ReadUInt64()
        $fs.Position = 54
        $phentsize = $br.ReadUInt16()
        $phnum = $br.ReadUInt16()
        $interp = $false; $dynamic = $false
        for ($i = 0; $i -lt $phnum; $i++) {
            $fs.Position = [int64]$phoff + [int64]$i * $phentsize
            switch ($br.ReadUInt32()) { 2 { $dynamic = $true } 3 { $interp = $true } }
        }
        # -s drops .symtab, -w the .debug_* (or .zdebug_*) sections.
        $fs.Position = 40
        $shoff = $br.ReadUInt64()
        $fs.Position = 58
        $shentsize = $br.ReadUInt16()
        $shnum = $br.ReadUInt16()
        $shstrndx = $br.ReadUInt16()
        $fs.Position = [int64]$shoff + [int64]$shstrndx * $shentsize + 24
        $namesAt = $br.ReadUInt64()
        $namesSize = $br.ReadUInt64()
        $fs.Position = [int64]$namesAt
        $names = $br.ReadBytes([int]$namesSize)
        $symtab = $false; $debug = $false
        for ($i = 0; $i -lt $shnum; $i++) {
            $fs.Position = [int64]$shoff + [int64]$i * $shentsize
            $nameAt = [int]$br.ReadUInt32()
            if ($br.ReadUInt32() -eq 2) { $symtab = $true }
            $end = [Array]::IndexOf($names, [byte]0, $nameAt)
            $name = if ($end -gt $nameAt) { [System.Text.Encoding]::ASCII.GetString($names, $nameAt, $end - $nameAt) } else { '' }
            if ($name.StartsWith('.debug') -or $name.StartsWith('.zdebug')) { $debug = $true }
        }
        [pscustomobject]@{ Is64 = ($ident[4] -eq 2); LittleEndian = ($ident[5] -eq 1); Executable = ($type -eq 2)
            X8664 = ($machine -eq 62); Static = (-not $interp -and -not $dynamic); Stripped = (-not $symtab -and -not $debug) }
    } finally {
        $fs.Dispose()
    }
}

function Get-BuildSettings([string]$Path) {
    <# The build settings recorded in a Go binary (go version -m), as a name -> value map. #>
    $lines = Get-GoLines -Arguments @('version', '-m', $Path)
    $map = @{}
    foreach ($l in $lines) {
        if ($l -match '^\s*build\s+([^=\s]+)=(.*)$') { $map[$Matches[1]] = $Matches[2].Trim().Trim('"') }
    }
    return $map
}

function Invoke-PackageStage {
    $packageScript = Join-Path $PSScriptRoot 'package.ps1'
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $dist = $DistDir
    $distNote = 'dashboard build'
    if (-not $dist) {
        $dist = Join-Path $NodesDir "placeholder-dist-$stamp"
        New-Item -ItemType Directory -Force -Path $dist | Out-Null
        [System.IO.File]::WriteAllText((Join-Path $dist 'index.html'),
            "<!doctype html><title>Zeal Clinic</title><p>Placeholder: the dashboard is not built in this checkout.</p>`n")
        $distNote = 'placeholder page'
    }
    $out = Join-Path $ScratchRoot "package-$stamp"
    $res = Invoke-ChildScript $packageScript @('-DistDir', $dist, '-OutDir', $out)
    $manifest = [System.IO.File]::ReadAllText($res.manifest) | ConvertFrom-Json
    Copy-Item -LiteralPath $res.manifest -Destination (Join-Path $ArtifactsDir 'manifest.json') -Force
    $version = $manifest.version
    $checks = [System.Collections.Generic.List[object]]::new()
    function Add-Check([string]$Name, [bool]$Pass, [string]$Detail) {
        $checks.Add([pscustomobject]@{ Name = $Name; Pass = $Pass; Detail = $Detail })
        Write-Host "$(if ($Pass) { 'ok  ' } else { 'FAIL' }) $Name`: $Detail"
    }

    # The installer's own call in the installed layout: the exe in Zeal Clinic\App, the database in Zeal Clinic\Data.
    $smoke = Join-Path $NodesDir "package-smoke-$stamp"
    $appData = Join-Path $smoke 'localappdata'
    $appDir = Join-Path $appData 'Zeal Clinic\App'
    $dataDir = Join-Path $appData 'Zeal Clinic\Data'
    $tmp = Join-Path $smoke 'tmp'
    New-Item -ItemType Directory -Force -Path $appDir, $dataDir, $tmp | Out-Null
    $installed = Join-Path $appDir 'ZealClinic.exe'
    Copy-Item -LiteralPath (Join-Path $out 'local\ZealClinic.exe') -Destination $installed
    $seedLog = Join-Path $ArtifactsDir 'seed-only.log'
    $run = Invoke-JailedProgram -FilePath $installed -Arguments @('--seed-only', '--no-browser') -WorkingDirectory $appDir `
        -LocalAppData $appData -Temp $tmp -LogPath $seedLog -TimeoutSec 180
    $migrations = @(Get-ChildItem -LiteralPath (Join-Path $RepoRoot 'internal/database/migrations') -File |
            Where-Object { $_.Name -match '^(\d{5})_.*\.(sql|go)$' } | ForEach-Object { [int]$Matches[1] })
    $latest = ($migrations | Measure-Object -Maximum).Maximum
    $db = Join-Path $dataDir 'clinic.db'
    $appLog = Join-Path $dataDir 'logs\clinic.log'
    $logText = if (Test-Path -LiteralPath $appLog) { [System.IO.File]::ReadAllText($appLog) } else { '' }
    $dbBytes = if (Test-Path -LiteralPath $db) { (Get-Item -LiteralPath $db).Length } else { 0 }
    $strays = @(@('tmp', 'data') | Where-Object { Test-Path -LiteralPath (Join-Path $appDir $_) })
    $migrated = $logText.Contains("goose: successfully migrated database to version: $latest")
    $stamped = $logText.Contains("release=$version")
    Add-Check 'seed-only in the installed layout' ((-not $run.TimedOut) -and $run.ExitCode -eq 0 -and $dbBytes -gt 0 -and $migrated -and $stamped -and
        -not $strays.Count -and $logText.Contains("Database initialized at $db")) `
        ("exit $($run.ExitCode)$(if ($run.TimedOut) { ' (timed out)' }), version $(if ($stamped) { $version } else { "? (want $version)" }), " +
        "clinic.db $([Math]::Round($dbBytes / 1KB)) KB in Zeal Clinic\Data, migrated to version $(if ($migrated) { $latest } else { "? (want $latest)" }), " +
        "$(if ($strays.Count) { "stray folders: $($strays -join ', ')" } else { 'no ./tmp or ./data next to the exe' })")
    if (Test-Path -LiteralPath $appLog) { Copy-Item -LiteralPath $appLog -Destination (Join-Path $ArtifactsDir 'seed-only-app.log') -Force }

    # A -trimpath build records no -ldflags in its build info, so their effects are checked on the file:
    # -s -w (no symbols, no DWARF), -H=windowsgui (GUI subsystem); the version shows in the seed run above.
    foreach ($f in @(
            @{ Path = 'local/ZealClinic.exe'; Subsystem = 'gui'; Trim = $true; Stripped = $true; Flags = $null },
            @{ Path = 'local/ZealUpdater.exe'; Subsystem = 'gui'; Trim = $true; Stripped = $true; Flags = $null },
            @{ Path = 'tools/ZealLegacyImport.exe'; Subsystem = 'console'; Trim = $false; Stripped = $false; Flags = "-X clinic-api/internal/buildmode.Version=$version-dev" })) {
        $full = Join-Path $out $f.Path
        $pe = Get-PeInfo $full
        $bs = Get-BuildSettings $full
        $ok = $pe -and $pe.Amd64 -and $pe.Subsystem -eq $f.Subsystem -and $pe.Stripped -eq $f.Stripped -and $bs['GOOS'] -eq 'windows' -and
        (($bs['-trimpath'] -eq 'true') -eq $f.Trim) -and ($null -eq $f.Flags -or $bs['-ldflags'] -eq $f.Flags)
        Add-Check $f.Path $ok ("PE $(if ($pe -and $pe.Amd64) { 'AMD64' } else { '?' }), $(if ($pe) { $pe.Subsystem } else { '?' }) subsystem, " +
            "$(if ($pe -and $pe.Stripped) { 'no symbols or DWARF' } else { 'symbols and DWARF kept' }), trimpath $($bs['-trimpath'] -eq 'true')" +
            "$(if ($f.Flags) { ", ldflags '$($bs['-ldflags'])'" })")
    }

    $cloudRel = "cloud/ZealClinicCloud-$version-linux-amd64"
    $cloudBin = Join-Path $out $cloudRel
    $elf = Get-ElfInfo $cloudBin
    $bs = Get-BuildSettings $cloudBin
    $ok = $elf -and $elf.Is64 -and $elf.LittleEndian -and $elf.Executable -and $elf.X8664 -and $elf.Static -and $elf.Stripped -and
    $bs['GOOS'] -eq 'linux' -and $bs['GOARCH'] -eq 'amd64' -and $bs['CGO_ENABLED'] -eq '0' -and $bs['-tags'] -eq 'cloud' -and $bs['-trimpath'] -eq 'true'
    Add-Check $cloudRel $ok ("ELF $(if ($elf -and $elf.Is64 -and $elf.X8664) { '64-bit x86-64' } else { '?' }), " +
        "$(if ($elf -and $elf.Static) { 'statically linked (no interpreter, no dynamic section)' } else { 'NOT static' }), " +
        "$(if ($elf -and $elf.Stripped) { 'no symbols or DWARF' } else { 'symbols or DWARF kept' }), " +
        "GOOS=$($bs['GOOS']) CGO_ENABLED=$($bs['CGO_ENABLED']) tags=$($bs['-tags']) trimpath=$($bs['-trimpath'])")

    foreach ($z in @(
            @{ Path = "local/ZealClinicUpdate-$version.zip"; Entries = @('ZealClinic.exe', 'ZealUpdater.exe') },
            @{ Path = "cloud/ZealClinicUpdate-$version-linux-amd64.zip"; Entries = @('ZealClinic') })) {
        $zipPath = Join-Path $out $z.Path
        $zip = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
        try { $names = @($zip.Entries | ForEach-Object FullName) } finally { $zip.Dispose() }
        $digest = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
        $recorded = [System.IO.File]::ReadAllText("$zipPath.sha256")
        $ok = (($names -join ',') -eq ($z.Entries -join ',')) -and $recorded -ceq $digest
        Add-Check $z.Path $ok ("entries $($names -join ', '); .sha256 $(if ($recorded -ceq $digest) { 'matches (lowercase hex, no newline)' } else { 'DOES NOT MATCH' })")
    }

    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add("package: version $version, dist: $distNote, output $out")
    $lines.Add('')
    foreach ($f in $manifest.files) { $lines.Add(('{0,14:N0}  {1}  {2}' -f $f.bytes, $f.sha256.Substring(0, 12), $f.path)) }
    $lines.Add('')
    foreach ($c in $checks) { $lines.Add("$(if ($c.Pass) { 'PASS' } else { 'FAIL' })  $($c.Name): $($c.Detail)") }
    $lines.Add('')
    $lines.Add('Not checked here: the Linux binary, which runs only on Linux (the Linux CI job starts it with --seed-only and checks /health),')
    $lines.Add('and the Inno Setup installer, which make installer builds only on Windows with Inno Setup installed.')
    [System.IO.File]::WriteAllLines((Join-Path $ArtifactsDir 'package.txt'), $lines)
    foreach ($l in $lines) { Write-Host $l }

    $sizes = @($manifest.files | Where-Object { $_.path -in 'local/ZealClinic.exe', $cloudRel } |
            ForEach-Object { '{0} {1:0.0} MB' -f (Split-Path -Leaf $_.path), ($_.bytes / 1MB) }) -join ', '
    $failedChecks = @($checks | Where-Object { -not $_.Pass })
    Write-Host ("Summary: package {0} ({1}): {2} files ({3}); smoke {4}/{5} checks passed{6}" -f $version, $distNote, @($manifest.files).Count,
        $sizes, ($checks.Count - $failedChecks.Count), $checks.Count, $(if ($failedChecks.Count) { "; FAILED: $(($failedChecks | ForEach-Object Name) -join ', ')" } else { '' }))
    return $(if ($failedChecks.Count) { 1 } else { 0 })
}

# Main

Push-Location $RepoRoot
$exitCode = 1
try {
    Write-Host "backend ci: stage $Stage, tag $Tags, artifacts $ArtifactsDir"
    $result = switch ($Stage) {
        'env-check' { Invoke-EnvCheckStage }
        'workflows' { Invoke-WorkflowsStage }
        'platform' { Invoke-PlatformStage }
        'vet' { Invoke-VetStage }
        'go-test' { Invoke-GoTestStage }
        'contract' { Invoke-ContractStage }
        'fmt' { Invoke-FmtStage }
        'mod' { Invoke-ModStage }
        'vuln' { Invoke-VulnStage }
        'secrets' { Invoke-SecretsStage }
        'system' { Invoke-SystemStage }
        'package' { Invoke-PackageStage }
    }
    $exitCode = [int](@($result)[-1])
} catch {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    $exitCode = 1
} finally {
    Pop-Location
}
exit $exitCode
