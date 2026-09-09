#Requires -Version 7.0
<#
.SYNOPSIS
    Builds and runs the server for end-to-end, system and quickstart tests.

.DESCRIPTION
    pwsh scripts/stack.ps1 -Build [-Tags default|cloud] [-Port <n>] [-PeerUrl <loopback url>] [-DistDir <dir>]
                           [-Version <stamp>] [-Name <binary name>]
    pwsh scripts/stack.ps1 -Start [-Tags default|cloud] [-Port <n>] [-PeerUrl <loopback url>] [-DistDir <dir>]
                           [-Demo] [-DataRoot <dir>] [-Binary <path>] [-TimeoutSec <n>]
    pwsh scripts/stack.ps1 -Stop -Pid <n>

    -Build  builds an unstamped console binary into the bin folder (never into build/) and prints one JSON
            line: binary, tags, port, version. -Version stamps a version instead (a node on another
            version for the system tests); -Name picks the binary name (default ZealClinicTest or
            ZealClinicCloudTest).
    -Start  builds (unless -Binary names a binary made by -Build), seeds the database with
            --dev --seed-only (plus --demo with -Demo), starts the server with --dev in its data folder,
            waits for /health and prints one JSON line: url, pid, port, dataRoot, logs.
    -Stop   stops a server started by -Start with Ctrl+Break, a graceful shutdown that closes the database
            cleanly. A hard stop is used only when Ctrl+Break can't be delivered, and is reported as such.

    Defaults: port 55580 (default tag) or 55582 (cloud tag); work folder "nodes" next to the checkout
    (RUNNER_TEMP on GitHub Actions); binaries in <work>/bin; data in <work>/<clinic|cloud>-<port>.
    -WorkDir and -BinDir override the folders; -Stop must see the same -BinDir as -Start.

    Safety: the same checkout guard as scripts/ci.ps1 (marker file or GitHub Actions; internal/config must
    hold exactly the committed files). Every build embeds throwaway config values, written into the
    git-ignored override files: random secrets and database key, the chosen port, an empty PEER_URL unless
    a loopback -PeerUrl is given, a loopback PUBLIC_URL. When the environment variables STACK_SYNC_SECRET,
    STACK_PUBLISH_SECRET and STACK_DB_ENCRYPTION_KEY (64 hex characters) are set, the build embeds them
    instead of random values, so two builds (a clinic and its cloud peer) can share them. The override
    files are removed right after the build. The Go tools and the server run with HTTP_PROXY and
    HTTPS_PROXY pointing at a closed local port and NO_PROXY unset: outbound HTTP calls fail at once while
    loopback traffic (health checks, a loopback peer) keeps working. The server's working folder,
    LOCALAPPDATA and TEMP are inside its data folder.
#>
[CmdletBinding()]
param(
    [Parameter(ParameterSetName = 'Build', Mandatory)][switch]$Build,
    [Parameter(ParameterSetName = 'Start', Mandatory)][switch]$Start,
    [Parameter(ParameterSetName = 'Stop', Mandatory)][switch]$Stop,
    [Parameter(ParameterSetName = 'Stop', Mandatory)][Alias('Pid')][int]$ServerPid,
    [Parameter(ParameterSetName = 'Build')][Parameter(ParameterSetName = 'Start')]
    [ValidateSet('default', 'cloud')][string]$Tags = 'default',
    [Parameter(ParameterSetName = 'Build')][Parameter(ParameterSetName = 'Start')]
    [int]$Port = 0,
    [Parameter(ParameterSetName = 'Build')][Parameter(ParameterSetName = 'Start')]
    [string]$PeerUrl = '',
    [Parameter(ParameterSetName = 'Build')][Parameter(ParameterSetName = 'Start')]
    [string]$DistDir,
    [Parameter(ParameterSetName = 'Build')][ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$')][string]$Version,
    [Parameter(ParameterSetName = 'Build')][ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')][string]$Name,
    [Parameter(ParameterSetName = 'Start')][switch]$Demo,
    [Parameter(ParameterSetName = 'Start')][string]$DataRoot,
    [Parameter(ParameterSetName = 'Start')][string]$Binary,
    [Parameter(ParameterSetName = 'Start')][int]$TimeoutSec = 90,
    [Parameter(ParameterSetName = 'Stop')][int]$StopTimeoutSec = 30,
    [string]$WorkDir,
    [string]$BinDir
)

# Guard: must run before anything else
$RepoRoot = Split-Path -Parent $PSScriptRoot
if ($env:GITHUB_ACTIONS -ne 'true' -and -not (Test-Path -LiteralPath (Join-Path $RepoRoot '.ci-scratch') -PathType Leaf)) {
    [Console]::Error.WriteLine("Refusing to run: '$RepoRoot' has no .ci-scratch marker at its root and this is not a GitHub Actions runner. The stack changes the checkout it runs in, so run it only in a throwaway clone, with an empty .ci-scratch file created at its root to confirm.")
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
if (-not $WorkDir) {
    $WorkDir = if ($OnActions -and $env:RUNNER_TEMP) { Join-Path $env:RUNNER_TEMP 'nodes' } else { Join-Path (Split-Path -Parent $RepoRoot) 'nodes' }
}
$WorkDir = [System.IO.Path]::GetFullPath($WorkDir)
if (-not $BinDir) { $BinDir = Join-Path $WorkDir 'bin' }
$BinDir = [System.IO.Path]::GetFullPath($BinDir)
$NodesDir = $WorkDir
$JailProxy = 'http://127.0.0.1:9'
# The git-ignored override file of each build; its committed dev defaults are <file>.defaults.
$ConfigFiles = [ordered]@{ default = 'internal/config/local.env'; cloud = 'internal/config/cloud.env' }
$DefaultPorts = @{ default = 55580; cloud = 55582 }
$ExeSuffix = if ($IsWindows) { '.exe' } else { '' }

function Test-PathUnder([string]$Path, [string]$Root) {
    $full = [System.IO.Path]::GetFullPath($Path)
    $r = [System.IO.Path]::GetFullPath($Root).TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    return $full.StartsWith($r, [System.StringComparison]::OrdinalIgnoreCase)
}

# Never put binaries or data inside the checkout's build/ folder (release outputs live there).
foreach ($p in @($WorkDir, $BinDir) + @($(if ($DataRoot) { $DataRoot }))) {
    if (Test-PathUnder $p (Join-Path $RepoRoot 'build')) {
        [Console]::Error.WriteLine("Refusing: $p is inside the build/ folder of the checkout.")
        exit 1
    }
}

# Helpers shared with scripts/ci.ps1 (keep in step)

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
      key after the committed dev defaults: random secrets (or the shared STACK_* ones), one sync secret and one
      database key for both builds, the given ports, loopback URLs. Restore-CommittedConfig removes the files.
    #>
    $sync = if ($env:STACK_SYNC_SECRET) { $env:STACK_SYNC_SECRET } else { New-RandomToken 48 }
    $publish = if ($env:STACK_PUBLISH_SECRET) { $env:STACK_PUBLISH_SECRET } else { New-RandomToken 48 }
    $dbKey = $env:STACK_DB_ENCRYPTION_KEY
    if (-not $dbKey) { $dbKey = New-RandomHexKey }
    elseif ($dbKey -notmatch '^[0-9A-Fa-f]{64}$') { throw 'STACK_DB_ENCRYPTION_KEY must be 64 hex characters' }
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

# Process control (Windows): new process group, own environment, output to a file

if ($IsWindows -and -not ('StackTools.Native' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

namespace StackTools
{
    public sealed class Child : IDisposable
    {
        [DllImport("kernel32.dll", SetLastError = true)] static extern uint WaitForSingleObject(IntPtr h, uint ms);
        [DllImport("kernel32.dll", SetLastError = true)] static extern bool GetExitCodeProcess(IntPtr h, out uint code);
        [DllImport("kernel32.dll", SetLastError = true)] static extern bool CloseHandle(IntPtr h);
        IntPtr handle;
        public int Pid { get; private set; }
        internal Child(int pid, IntPtr h) { Pid = pid; handle = h; }
        public bool WaitForExit(int milliseconds) { return WaitForSingleObject(handle, (uint)milliseconds) == 0; }
        public bool HasExited { get { return WaitForSingleObject(handle, 0) == 0; } }
        public int ExitCode { get { uint c; GetExitCodeProcess(handle, out c); return unchecked((int)c); } }
        public void Dispose() { if (handle != IntPtr.Zero) { CloseHandle(handle); handle = IntPtr.Zero; } }
    }

    public static class Native
    {
        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        struct STARTUPINFO
        {
            public int cb; public IntPtr lpReserved; public IntPtr lpDesktop; public IntPtr lpTitle;
            public int dwX; public int dwY; public int dwXSize; public int dwYSize;
            public int dwXCountChars; public int dwYCountChars; public int dwFillAttribute; public int dwFlags;
            public short wShowWindow; public short cbReserved2; public IntPtr lpReserved2;
            public IntPtr hStdInput; public IntPtr hStdOutput; public IntPtr hStdError;
        }
        [StructLayout(LayoutKind.Sequential)]
        struct STARTUPINFOEX { public STARTUPINFO StartupInfo; public IntPtr lpAttributeList; }
        [StructLayout(LayoutKind.Sequential)]
        struct PROCESS_INFORMATION { public IntPtr hProcess; public IntPtr hThread; public int dwProcessId; public int dwThreadId; }
        [StructLayout(LayoutKind.Sequential)]
        struct SECURITY_ATTRIBUTES { public int nLength; public IntPtr lpSecurityDescriptor; public int bInheritHandle; }

        [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        static extern bool CreateProcessW(string app, StringBuilder cmd, IntPtr pa, IntPtr ta, bool inherit, uint flags,
                                          IntPtr env, string cwd, ref STARTUPINFOEX si, out PROCESS_INFORMATION pi);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool InitializeProcThreadAttributeList(IntPtr list, int count, int flags, ref IntPtr size);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool UpdateProcThreadAttribute(IntPtr list, uint flags, IntPtr attr, IntPtr value, IntPtr size, IntPtr prev, IntPtr ret);
        [DllImport("kernel32.dll")]
        static extern void DeleteProcThreadAttributeList(IntPtr list);
        [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        static extern IntPtr CreateFileW(string name, uint access, uint share, ref SECURITY_ATTRIBUTES sa, uint disposition, uint flags, IntPtr template);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool CloseHandle(IntPtr h);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool GenerateConsoleCtrlEvent(uint ev, uint group);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern uint GetConsoleProcessList(uint[] list, uint count);

        const uint CREATE_NEW_PROCESS_GROUP = 0x00000200;
        const uint CREATE_UNICODE_ENVIRONMENT = 0x00000400;
        const uint CREATE_NO_WINDOW = 0x08000000;
        const uint EXTENDED_STARTUPINFO_PRESENT = 0x00080000;
        const int STARTF_USESTDHANDLES = 0x00000100;
        const uint GENERIC_READ = 0x80000000;
        const uint GENERIC_WRITE = 0x40000000;
        const uint FILE_SHARE_ALL = 0x7;
        const uint OPEN_EXISTING = 3;
        const uint CREATE_ALWAYS = 2;
        const uint FILE_ATTRIBUTE_NORMAL = 0x80;
        static readonly IntPtr INVALID_HANDLE_VALUE = new IntPtr(-1);
        static readonly IntPtr PROC_THREAD_ATTRIBUTE_HANDLE_LIST = new IntPtr(0x00020002);

        public static bool HasConsole()
        {
            var buf = new uint[1];
            return GetConsoleProcessList(buf, 1) > 0;
        }

        public static bool SharesConsoleWith(int pid)
        {
            var buf = new uint[4096];
            uint n = GetConsoleProcessList(buf, (uint)buf.Length);
            if (n == 0 || n > buf.Length) return false;
            for (int i = 0; i < n; i++) if (buf[i] == (uint)pid) return true;
            return false;
        }

        // CTRL_BREAK_EVENT to the process group whose id is pid.
        public static bool SendCtrlBreak(int pid) { return GenerateConsoleCtrlEvent(1, (uint)pid); }

        // Starts exe in a new process group (so Ctrl+Break can reach it alone) with the given environment.
        // stdout and stderr go to logPath, stdin is NUL, and only those two handles are inherited, so the
        // caller's own pipes never stay open because of the child.
        public static Child Start(string exe, string[] args, string workDir, IDictionary env, string logPath)
        {
            var cmd = new StringBuilder(Quote(exe));
            foreach (var a in args) cmd.Append(' ').Append(Quote(a));

            var names = new List<string>();
            foreach (DictionaryEntry e in env) names.Add(e.Key.ToString());
            names.Sort(StringComparer.OrdinalIgnoreCase);
            var block = new StringBuilder();
            foreach (var n in names)
            {
                var v = env[n];
                if (v == null || n.Length == 0 || n[0] == '=') continue;
                block.Append(n).Append('=').Append(v.ToString()).Append('\0');
            }
            block.Append('\0');

            var sa = new SECURITY_ATTRIBUTES { nLength = Marshal.SizeOf(typeof(SECURITY_ATTRIBUTES)), bInheritHandle = 1 };
            IntPtr hLog = CreateFileW(logPath, GENERIC_WRITE, FILE_SHARE_ALL, ref sa, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, IntPtr.Zero);
            if (hLog == INVALID_HANDLE_VALUE) throw new Win32Exception(Marshal.GetLastWin32Error(), "open " + logPath);
            IntPtr hNul = CreateFileW("NUL", GENERIC_READ, FILE_SHARE_ALL, ref sa, OPEN_EXISTING, 0, IntPtr.Zero);
            if (hNul == INVALID_HANDLE_VALUE)
            {
                int err = Marshal.GetLastWin32Error();
                CloseHandle(hLog);
                throw new Win32Exception(err, "open NUL");
            }

            IntPtr envPtr = IntPtr.Zero, attrList = IntPtr.Zero, handles = IntPtr.Zero;
            bool attrInit = false;
            try
            {
                envPtr = Marshal.StringToHGlobalUni(block.ToString());
                IntPtr size = IntPtr.Zero;
                InitializeProcThreadAttributeList(IntPtr.Zero, 1, 0, ref size);
                attrList = Marshal.AllocHGlobal(size);
                if (!InitializeProcThreadAttributeList(attrList, 1, 0, ref size))
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "InitializeProcThreadAttributeList");
                attrInit = true;
                handles = Marshal.AllocHGlobal(IntPtr.Size * 2);
                Marshal.WriteIntPtr(handles, 0, hLog);
                Marshal.WriteIntPtr(handles, IntPtr.Size, hNul);
                if (!UpdateProcThreadAttribute(attrList, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST, handles,
                                               (IntPtr)(IntPtr.Size * 2), IntPtr.Zero, IntPtr.Zero))
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "UpdateProcThreadAttribute");

                var si = new STARTUPINFOEX();
                si.StartupInfo.cb = Marshal.SizeOf(typeof(STARTUPINFOEX));
                si.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
                si.StartupInfo.hStdInput = hNul;
                si.StartupInfo.hStdOutput = hLog;
                si.StartupInfo.hStdError = hLog;
                si.lpAttributeList = attrList;

                uint flags = CREATE_NEW_PROCESS_GROUP | CREATE_UNICODE_ENVIRONMENT | EXTENDED_STARTUPINFO_PRESENT;
                if (!HasConsole()) flags |= CREATE_NO_WINDOW;
                PROCESS_INFORMATION pi;
                if (!CreateProcessW(exe, cmd, IntPtr.Zero, IntPtr.Zero, true, flags, envPtr, workDir, ref si, out pi))
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "CreateProcess " + exe);
                CloseHandle(pi.hThread);
                return new Child(pi.dwProcessId, pi.hProcess);
            }
            finally
            {
                if (attrInit) DeleteProcThreadAttributeList(attrList);
                if (attrList != IntPtr.Zero) Marshal.FreeHGlobal(attrList);
                if (handles != IntPtr.Zero) Marshal.FreeHGlobal(handles);
                if (envPtr != IntPtr.Zero) Marshal.FreeHGlobal(envPtr);
                CloseHandle(hLog);
                CloseHandle(hNul);
            }
        }

        static string Quote(string s)
        {
            if (s.Length > 0 && s.IndexOfAny(new[] { ' ', '\t', '"' }) < 0) return s;
            var sb = new StringBuilder("\"");
            int backslashes = 0;
            foreach (char c in s)
            {
                if (c == '\\') { backslashes++; continue; }
                if (c == '"') { sb.Append('\\', backslashes * 2 + 1).Append('"'); backslashes = 0; continue; }
                sb.Append('\\', backslashes).Append(c);
                backslashes = 0;
            }
            sb.Append('\\', backslashes * 2).Append('"');
            return sb.ToString();
        }
    }
}
'@
}

# Stack helpers

function Write-JsonLine([System.Collections.IDictionary]$Data) {
    # Straight to stdout (not the pipeline) so callers can parse the last line that starts with '{'.
    [Console]::Out.WriteLine(($Data | ConvertTo-Json -Compress -Depth 4))
    [Console]::Out.Flush()
}

function Get-LogTail([string]$Path, [int]$Lines = 30) {
    if (Test-Path -LiteralPath $Path) { return (Get-Content -LiteralPath $Path -Tail $Lines) -join [Environment]::NewLine }
    return '(no log)'
}

function Test-PortListening([int]$P) {
    $listeners = [System.Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners()
    return @($listeners | Where-Object { $_.Port -eq $P }).Count -gt 0
}

function Resolve-Port {
    if ($Port -eq 0) { return $DefaultPorts[$Tags] }
    if ($Port -lt 1024 -or $Port -gt 65535) { throw "invalid -Port $Port (use 1024-65535)" }
    return $Port
}

function Assert-Platform {
    if ($Tags -eq 'default' -and -not $IsWindows) {
        [Console]::Error.WriteLine('The default tag builds the clinic (local) server, which is Windows-only by design. Use -Tags cloud on this OS.')
        exit 1
    }
    if ($PeerUrl) {
        $u = $null
        if (-not [Uri]::TryCreate($PeerUrl, [UriKind]::Absolute, [ref]$u) -or $u.Scheme -notin 'http', 'https' -or -not $u.IsLoopback) {
            [Console]::Error.WriteLine('Refusing -PeerUrl: only http(s) URLs on a loopback host (127.0.0.1, ::1, localhost) are allowed.')
            exit 1
        }
        if ($Tags -ne 'default') {
            [Console]::Error.WriteLine('Refusing -PeerUrl: only the clinic (default tag) build has a peer; the cloud build keeps it empty.')
            exit 1
        }
    }
}

function Copy-Dist {
    if (-not $DistDir) { if (Initialize-ClientDist) { return 'placeholder' } else { return 'existing client/dist' } }
    $src = [System.IO.Path]::GetFullPath($DistDir)
    if (-not (Test-Path -LiteralPath (Join-Path $src 'index.html') -PathType Leaf)) { throw "-DistDir has no index.html: $src" }
    $dst = Join-Path $RepoRoot 'client/dist'
    New-Item -ItemType Directory -Force -Path $dst | Out-Null
    Copy-Item -Path (Join-Path $src '*') -Destination $dst -Recurse -Force
    Write-Host "client/dist: copied from $src"
    return 'dashboard build'
}

function Invoke-ServerBuild([int]$ServerPort) {
    Assert-Platform
    $name = if ($Name) { $Name } elseif ($Tags -eq 'cloud') { 'ZealClinicCloudTest' } else { 'ZealClinicTest' }
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $out = Join-Path $BinDir ($name + $ExeSuffix)
    Initialize-GoModules
    $dist = Copy-Dist
    $overrides = @{ default = $DefaultPorts.default; cloud = $DefaultPorts.cloud }
    $overrides[$Tags] = $ServerPort
    $code = 99
    Write-TestConfig -PortOverrides $overrides -Peer $PeerUrl
    try {
        Enter-GoJail
        $goArgs = @('build', '-o', $out)
        if ($Tags -eq 'cloud') { $goArgs += @('-tags', 'cloud') }
        if ($Version) { $goArgs += @('-ldflags', "-X clinic-api/internal/buildmode.Version=$Version") }
        $goArgs += './cmd/server'
        Write-Host "> go $($goArgs -join ' ')   ($(if ($Version) { "console build stamped $Version" } else { 'unstamped console build' }))"
        $global:LASTEXITCODE = 99
        & go @goArgs | Out-Host
        $code = $LASTEXITCODE
    } finally {
        Exit-GoJail
        $restored = Restore-CommittedConfig
    }
    if ($code -ne 0) { throw "go build failed (exit $code)" }
    if (-not $restored) { throw 'config restore failed' }
    $info = [ordered]@{ binary = $out; tags = $Tags; port = $ServerPort; version = $(if ($Version) { $Version } else { 'dev' })
        peer = $(if ($PeerUrl) { 'loopback' } else { 'empty' }); dist = $dist; built = (Get-Date).ToString('o') }
    Set-Content -LiteralPath "$out.json" -Encoding utf8 -Value ($info | ConvertTo-Json)
    return $info
}

function New-NodeEnvironment([string]$Root) {
    $table = [System.Collections.Hashtable]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($e in [Environment]::GetEnvironmentVariables().GetEnumerator()) { $table[[string]$e.Key] = [string]$e.Value }
    foreach ($k in 'NO_PROXY', 'no_proxy', 'GOFLAGS', 'GOPROXY') { $table.Remove($k) }
    $local = Join-Path $Root 'localappdata'
    $tmp = Join-Path $Root 'tmp-env'
    New-Item -ItemType Directory -Force -Path $local, $tmp | Out-Null
    $table['HTTP_PROXY'] = $JailProxy
    $table['HTTPS_PROXY'] = $JailProxy
    $table['LOCALAPPDATA'] = $local
    if ($IsWindows) { $table['TEMP'] = $tmp; $table['TMP'] = $tmp } else { $table['TMPDIR'] = $tmp; $table['http_proxy'] = $JailProxy; $table['https_proxy'] = $JailProxy }
    return $table
}

function Start-Node([string]$Exe, [string[]]$Arguments, [string]$Root, [hashtable]$NodeEnv, [string]$LogPath) {
    <# Returns an object with Pid, WaitForExit(ms), HasExited, ExitCode, Dispose(). #>
    if ($IsWindows) {
        return [StackTools.Native]::Start($Exe, [string[]]$Arguments, $Root, $NodeEnv, $LogPath)
    }
    # Other systems: sh redirects the output to the log and execs the server (same pid).
    $saved = @{}
    foreach ($k in @($NodeEnv.Keys)) { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $NodeEnv[$k]) }
    foreach ($k in 'NO_PROXY', 'no_proxy') { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $null) }
    try {
        $quoted = ($Arguments | ForEach-Object { "'$_'" }) -join ' '
        $psi = [System.Diagnostics.ProcessStartInfo]::new('/bin/sh')
        foreach ($a in '-c', "exec '$Exe' $quoted >'$LogPath' 2>&1 </dev/null") { $psi.ArgumentList.Add($a) }
        $psi.UseShellExecute = $false
        $psi.WorkingDirectory = $Root
        $p = [System.Diagnostics.Process]::Start($psi)
        return [pscustomobject]@{ Pid = $p.Id; Process = $p } |
            Add-Member -MemberType ScriptMethod -Name WaitForExit -Value { param($ms) $this.Process.WaitForExit($ms) } -PassThru |
            Add-Member -MemberType ScriptProperty -Name HasExited -Value { $this.Process.HasExited } -PassThru |
            Add-Member -MemberType ScriptProperty -Name ExitCode -Value { $this.Process.ExitCode } -PassThru |
            Add-Member -MemberType ScriptMethod -Name Dispose -Value { } -PassThru
    } finally {
        foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
    }
}

function Wait-Health([int]$ServerPort, $Child, [int]$Seconds) {
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($Child.HasExited) { return [pscustomobject]@{ Ok = $false; Reason = "the server exited (code $($Child.ExitCode))"; Version = $null } }
        try {
            $r = Invoke-RestMethod -Uri "http://127.0.0.1:$ServerPort/health" -NoProxy -TimeoutSec 3
            if ($r.status -eq 'ok') { return [pscustomobject]@{ Ok = $true; Reason = ''; Version = $r.version } }
        } catch { }
        Start-Sleep -Milliseconds 300
    }
    return [pscustomobject]@{ Ok = $false; Reason = "no healthy answer within $Seconds s"; Version = $null }
}

function Send-BreakFromHelper([int]$TargetPid) {
    # A short-lived helper attaches to the server's console and sends Ctrl+Break to the server's group only.
    $code = @"
`$sig = @'
[DllImport("kernel32.dll", SetLastError = true)] public static extern bool FreeConsole();
[DllImport("kernel32.dll", SetLastError = true)] public static extern bool AttachConsole(uint p);
[DllImport("kernel32.dll", SetLastError = true)] public static extern bool SetConsoleCtrlHandler(System.IntPtr h, bool a);
[DllImport("kernel32.dll", SetLastError = true)] public static extern bool GenerateConsoleCtrlEvent(uint e, uint g);
'@
`$k = Add-Type -MemberDefinition `$sig -Name BreakSender -Namespace StackBreak -PassThru
`$null = `$k::FreeConsole()
if (-not `$k::AttachConsole($TargetPid)) { exit 2 }
`$null = `$k::SetConsoleCtrlHandler([System.IntPtr]::Zero, `$true)
if (`$k::GenerateConsoleCtrlEvent(1, $TargetPid)) { exit 0 } else { exit 3 }
"@
    $enc = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($code))
    $psi = [System.Diagnostics.ProcessStartInfo]::new([Environment]::ProcessPath)
    foreach ($a in '-NoProfile', '-NonInteractive', '-EncodedCommand', $enc) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    if (-not $p.WaitForExit(30000)) { try { $p.Kill() } catch { }; return $false }
    return ($p.ExitCode -eq 0)
}

# Modes

function Invoke-BuildMode {
    $serverPort = Resolve-Port
    $info = Invoke-ServerBuild -ServerPort $serverPort
    Write-JsonLine ([ordered]@{ binary = $info.binary; tags = $info.tags; port = $info.port; version = $info.version; peer = $info.peer; dist = $info.dist })
    return 0
}

function Invoke-StartMode {
    Assert-Platform
    if ($Binary) {
        $bin = [System.IO.Path]::GetFullPath($Binary)
        if (-not (Test-Path -LiteralPath $bin -PathType Leaf)) { throw "-Binary not found: $bin" }
        if (-not (Test-PathUnder $bin $BinDir)) { throw "-Binary must be a build made by -Build under $BinDir" }
        $meta = Get-Content -LiteralPath "$bin.json" -Raw | ConvertFrom-Json
        if ($meta.tags -ne $Tags) { throw "-Binary was built with -Tags $($meta.tags), not $Tags" }
        if ($Port -ne 0 -and $Port -ne [int]$meta.port) { throw "-Binary was built for port $($meta.port), not $Port" }
        $serverPort = [int]$meta.port
    } else {
        $serverPort = Resolve-Port
        $bin = (Invoke-ServerBuild -ServerPort $serverPort).binary
    }

    if (Test-PortListening $serverPort) { throw "port $serverPort is already in use" }
    if ($IsWindows -and $Tags -eq 'default') {
        $m = $null
        if ([System.Threading.Mutex]::TryOpenExisting('ZealClinic-singleton-lock', [ref]$m)) {
            $m.Dispose()
            throw 'another clinic server is running on this machine (single-instance lock held); stop it first'
        }
    }

    if (-not $DataRoot) {
        $DataRoot = Join-Path $WorkDir ('{0}-{1}' -f $(if ($Tags -eq 'cloud') { 'cloud' } else { 'clinic' }), $serverPort)
    }
    $root = [System.IO.Path]::GetFullPath($DataRoot)
    New-Item -ItemType Directory -Force -Path $root | Out-Null
    $nodeEnv = New-NodeEnvironment -Root $root
    $appLog = Join-Path $root $(if ($Tags -eq 'cloud') { 'data/logs/clinic.log' } else { 'tmp/logs/clinic.log' })

    # 1. Seed (migrations, plus demo data on request), then exit.
    $seedArgs = @('--dev', '--seed-only')
    if ($Demo) { $seedArgs += '--demo' }
    $seedLog = Join-Path $root 'seed-console.log'
    Write-Host "> $(Split-Path -Leaf $bin) $($seedArgs -join ' ')   (in $root)"
    $seed = Start-Node -Exe $bin -Arguments $seedArgs -Root $root -NodeEnv $nodeEnv -LogPath $seedLog
    try {
        if (-not $seed.WaitForExit(600000)) { throw 'seeding did not finish within 10 minutes' }
        if ($seed.ExitCode -ne 0) { throw "seeding failed (exit $($seed.ExitCode)):`n$(Get-LogTail $seedLog)" }
    } finally { $seed.Dispose() }
    Write-Host "seed: done$(if ($Demo) { ' (demo data)' })"

    # 2. Start the server.
    $consoleLog = Join-Path $root 'server-console.log'
    Write-Host "> $(Split-Path -Leaf $bin) --dev   (in $root, port $serverPort)"
    $child = Start-Node -Exe $bin -Arguments @('--dev') -Root $root -NodeEnv $nodeEnv -LogPath $consoleLog
    $serverPid = $child.Pid
    try {
        $health = Wait-Health -ServerPort $serverPort -Child $child -Seconds $TimeoutSec
        if (-not $health.Ok) {
            if (-not $child.HasExited) { Stop-Process -Id $serverPid -Force -ErrorAction SilentlyContinue }
            throw "server not healthy: $($health.Reason)`n$(Get-LogTail $consoleLog)"
        }
    } finally { $child.Dispose() }

    $state = [ordered]@{
        url      = "http://127.0.0.1:$serverPort"
        pid      = $serverPid
        port     = $serverPort
        tags     = $Tags
        version  = $health.Version
        demo     = [bool]$Demo
        dataRoot = $root
        binary   = $bin
        log      = $appLog
        console  = $consoleLog
    }
    Set-Content -LiteralPath (Join-Path $root 'stack-state.json') -Encoding utf8 -Value ($state | ConvertTo-Json)
    Write-Host "server: healthy at $($state.url) (pid $serverPid, version $($health.Version))"
    Write-JsonLine $state
    return 0
}

function Invoke-StopMode {
    $proc = Get-Process -Id $ServerPid -ErrorAction SilentlyContinue
    if (-not $proc) {
        Write-JsonLine ([ordered]@{ pid = $ServerPid; stopped = $true; graceful = $null; method = 'none'; note = 'not running' })
        return 0
    }
    $path = try { $proc.Path } catch { $null }
    if (-not $path -or -not (Test-PathUnder $path $BinDir)) {
        [Console]::Error.WriteLine("Refusing to stop pid ${ServerPid}: it is not a server binary under $BinDir")
        return 1
    }
    try { $null = $proc.Handle } catch { }   # open the handle now so the exit code stays readable after exit
    $sent = $false
    $method = ''
    if ($IsWindows) {
        if ([StackTools.Native]::SharesConsoleWith($ServerPid)) {
            $method = 'ctrl-break'
            $sent = [StackTools.Native]::SendCtrlBreak($ServerPid)
        } else {
            $method = 'ctrl-break via helper'
            $sent = Send-BreakFromHelper -TargetPid $ServerPid
        }
    } else {
        $method = 'SIGINT'
        $global:LASTEXITCODE = 99
        & /bin/kill -s INT $ServerPid
        $sent = ($LASTEXITCODE -eq 0)
    }
    $graceful = $false
    if ($sent) { $graceful = $proc.WaitForExit($StopTimeoutSec * 1000) }
    if (-not $graceful) {
        Write-Warning "graceful stop failed ($method$(if (-not $sent) { ': could not be delivered' } else { ': no exit in time' })); stopping pid $ServerPid hard - NOT a graceful shutdown"
        Stop-Process -Id $ServerPid -Force -ErrorAction SilentlyContinue
        $null = $proc.WaitForExit(15000)
        $method = "$method, then hard stop"
    }
    $exit = $null
    try { $exit = $proc.ExitCode } catch { }
    Write-Host "server: pid $ServerPid stopped ($method)"
    Write-JsonLine ([ordered]@{ pid = $ServerPid; stopped = $proc.HasExited; graceful = $graceful; method = $method; exitCode = $exit })
    return $(if ($proc.HasExited) { 0 } else { 1 })
}

# Main

Push-Location $RepoRoot
$exitCode = 1
try {
    $result = switch ($PSCmdlet.ParameterSetName) {
        'Build' { Invoke-BuildMode }
        'Start' { Invoke-StartMode }
        'Stop' { Invoke-StopMode }
    }
    $exitCode = [int](@($result)[-1])
} catch {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    $exitCode = 1
} finally {
    Pop-Location
}
exit $exitCode
