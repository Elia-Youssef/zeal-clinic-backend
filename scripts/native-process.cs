// Windows process helpers of the PowerShell scripts in this folder (stack.ps1 and the demo scripts), loaded with
// Add-Type -Path:
// - ConsoleProcess starts a program in its own process group on the caller's console, with its output in a log file
//   or on the console, and stops it with Ctrl+Break, directly or through a short-lived helper when it runs on another
//   console;
// - a started program can be tied to the caller, so that Windows ends it when the caller ends, however it ends;
// - ConsoleEvents records Ctrl+C, Ctrl+Break and a closing console instead of letting them end the caller.
using System;
using System.Collections;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

namespace ZealScripts
{
    // A program started by ConsoleProcess.Start. It keeps the process handle, so the exit code stays readable after
    // the process has ended.
    public sealed class ChildProcess : IDisposable
    {
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern uint WaitForSingleObject(IntPtr handle, uint milliseconds);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool GetExitCodeProcess(IntPtr handle, out uint code);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool TerminateProcess(IntPtr handle, uint code);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool CloseHandle(IntPtr handle);

        const uint WAIT_OBJECT_0 = 0;
        IntPtr handle;

        internal ChildProcess(int pid, IntPtr processHandle, bool endsWithCaller)
        {
            Pid = pid;
            handle = processHandle;
            EndsWithCaller = endsWithCaller;
        }

        public int Pid { get; private set; }
        // True when Windows ends the program together with the caller (see ConsoleProcess.Start).
        public bool EndsWithCaller { get; private set; }
        public bool HasExited { get { return WaitForExit(0); } }
        public int ExitCode { get { uint code; GetExitCodeProcess(handle, out code); return unchecked((int)code); } }

        public bool WaitForExit(int milliseconds)
        {
            return WaitForSingleObject(handle, (uint)milliseconds) == WAIT_OBJECT_0;
        }

        // A hard stop, ending the process and its whole process tree. When that throws (for example because the
        // process has just exited), the fallback ends only the process itself.
        public void Kill()
        {
            if (handle == IntPtr.Zero) return;
            try
            {
                using (var p = Process.GetProcessById(Pid))
                {
                    p.Kill(true);
                }
            }
            catch
            {
                TerminateProcess(handle, 1);
            }
        }

        public void Dispose()
        {
            if (handle == IntPtr.Zero) return;
            CloseHandle(handle);
            handle = IntPtr.Zero;
        }
    }

    public static class ConsoleProcess
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
        struct PROCESS_INFORMATION
        {
            public IntPtr hProcess; public IntPtr hThread; public int dwProcessId; public int dwThreadId;
        }
        [StructLayout(LayoutKind.Sequential)]
        struct SECURITY_ATTRIBUTES
        {
            public int nLength; public IntPtr lpSecurityDescriptor; public int bInheritHandle;
        }

        [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        static extern bool CreateProcessW(string app, StringBuilder cmd, IntPtr processAttributes,
                                          IntPtr threadAttributes, bool inherit, uint flags, IntPtr environment,
                                          string workDir, ref STARTUPINFOEX startupInfo,
                                          out PROCESS_INFORMATION processInfo);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool InitializeProcThreadAttributeList(IntPtr list, int count, int flags, ref IntPtr size);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool UpdateProcThreadAttribute(IntPtr list, uint flags, IntPtr attribute, IntPtr value,
                                                     IntPtr size, IntPtr previous, IntPtr returned);
        [DllImport("kernel32.dll")]
        static extern void DeleteProcThreadAttributeList(IntPtr list);
        [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        static extern IntPtr CreateFileW(string name, uint access, uint share,
                                         ref SECURITY_ATTRIBUTES securityAttributes, uint disposition, uint flags,
                                         IntPtr template);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool CloseHandle(IntPtr handle);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern uint ResumeThread(IntPtr thread);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool TerminateProcess(IntPtr process, uint code);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool GenerateConsoleCtrlEvent(uint consoleEvent, uint processGroup);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern uint GetConsoleProcessList(uint[] list, uint count);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool FreeConsole();
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool AttachConsole(uint pid);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool SetConsoleCtrlHandler(IntPtr handler, bool add);

        const uint CREATE_SUSPENDED = 0x00000004;
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
        const uint RESUME_FAILED = 0xFFFFFFFF;
        const uint CTRL_BREAK_EVENT = 1;
        static readonly IntPtr INVALID_HANDLE_VALUE = new IntPtr(-1);
        static readonly IntPtr PROC_THREAD_ATTRIBUTE_HANDLE_LIST = new IntPtr(0x00020002);

        // Exit codes of SendCtrlBreakFromAttachedConsole, the helper's work.
        public const int HelperSent = 0;
        public const int HelperCannotAttach = 2;
        public const int HelperCannotSend = 3;

        // Starts exe in a new process group on the caller's console (a hidden one when the caller has none).
        // When logPath is set (created new), stdout and stderr are written there, stdin is NUL, and only those
        // two handles are inherited, so a pipe the caller writes to never stays open because of the program.
        // When logPath is null or empty, the program inherits the caller's standard handles and inheritable handles.
        // environment replaces the caller's environment when it isn't null. With endWithCaller the program is started
        // suspended, put into the caller's job (see CallerJob) and then resumed, so neither it nor anything it starts
        // outlives the caller.
        public static ChildProcess Start(string exe, string[] args, string workDir, IDictionary environment,
                                         string logPath, bool endWithCaller)
        {
            var cmd = new StringBuilder(Quote(exe));
            foreach (var a in args) cmd.Append(' ').Append(Quote(a));

            bool hasLog = !string.IsNullOrEmpty(logPath);
            IntPtr hLog = IntPtr.Zero, hNul = IntPtr.Zero;
            if (hasLog)
            {
                var sa = new SECURITY_ATTRIBUTES();
                sa.nLength = Marshal.SizeOf(typeof(SECURITY_ATTRIBUTES));
                sa.bInheritHandle = 1;
                hLog = CreateFileW(logPath, GENERIC_WRITE, FILE_SHARE_ALL, ref sa, CREATE_ALWAYS,
                                   FILE_ATTRIBUTE_NORMAL, IntPtr.Zero);
                if (hLog == INVALID_HANDLE_VALUE)
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "open " + logPath);
                hNul = CreateFileW("NUL", GENERIC_READ, FILE_SHARE_ALL, ref sa, OPEN_EXISTING, 0, IntPtr.Zero);
                if (hNul == INVALID_HANDLE_VALUE)
                {
                    int err = Marshal.GetLastWin32Error();
                    CloseHandle(hLog);
                    throw new Win32Exception(err, "open NUL");
                }
            }

            IntPtr envBlock = IntPtr.Zero, attrList = IntPtr.Zero, handles = IntPtr.Zero;
            bool attrInit = false;
            try
            {
                uint flags = CREATE_NEW_PROCESS_GROUP | EXTENDED_STARTUPINFO_PRESENT;
                if (!HasConsole()) flags |= CREATE_NO_WINDOW;
                if (endWithCaller) flags |= CREATE_SUSPENDED;
                if (environment != null)
                {
                    envBlock = Marshal.StringToHGlobalUni(EnvironmentBlock(environment));
                    flags |= CREATE_UNICODE_ENVIRONMENT;
                }

                int attrCount = hasLog ? 1 : 0;
                IntPtr size = IntPtr.Zero;
                InitializeProcThreadAttributeList(IntPtr.Zero, attrCount, 0, ref size);
                attrList = Marshal.AllocHGlobal(size);
                if (!InitializeProcThreadAttributeList(attrList, attrCount, 0, ref size))
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "InitializeProcThreadAttributeList");
                attrInit = true;

                if (hasLog)
                {
                    handles = Marshal.AllocHGlobal(IntPtr.Size * 2);
                    Marshal.WriteIntPtr(handles, 0, hLog);
                    Marshal.WriteIntPtr(handles, IntPtr.Size, hNul);
                    if (!UpdateProcThreadAttribute(attrList, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST, handles,
                                                   (IntPtr)(IntPtr.Size * 2), IntPtr.Zero, IntPtr.Zero))
                        throw new Win32Exception(Marshal.GetLastWin32Error(), "UpdateProcThreadAttribute");
                }

                var si = new STARTUPINFOEX();
                si.StartupInfo.cb = Marshal.SizeOf(typeof(STARTUPINFOEX));
                if (hasLog)
                {
                    si.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
                    si.StartupInfo.hStdInput = hNul;
                    si.StartupInfo.hStdOutput = hLog;
                    si.StartupInfo.hStdError = hLog;
                }
                si.lpAttributeList = attrList;

                PROCESS_INFORMATION pi;
                if (!CreateProcessW(exe, cmd, IntPtr.Zero, IntPtr.Zero, true, flags, envBlock, workDir, ref si, out pi))
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "CreateProcess " + exe);
                bool endsWithCaller = false;
                if (endWithCaller)
                {
                    endsWithCaller = CallerJob.Add(pi.hProcess);
                    if (ResumeThread(pi.hThread) == RESUME_FAILED)
                    {
                        int err = Marshal.GetLastWin32Error();
                        TerminateProcess(pi.hProcess, 1);
                        CloseHandle(pi.hThread);
                        CloseHandle(pi.hProcess);
                        throw new Win32Exception(err, "ResumeThread " + exe);
                    }
                }
                CloseHandle(pi.hThread);
                return new ChildProcess(pi.dwProcessId, pi.hProcess, endsWithCaller);
            }
            finally
            {
                if (attrInit) DeleteProcThreadAttributeList(attrList);
                if (attrList != IntPtr.Zero) Marshal.FreeHGlobal(attrList);
                if (handles != IntPtr.Zero) Marshal.FreeHGlobal(handles);
                if (envBlock != IntPtr.Zero) Marshal.FreeHGlobal(envBlock);
                if (hLog != IntPtr.Zero) CloseHandle(hLog);
                if (hNul != IntPtr.Zero) CloseHandle(hNul);
            }
        }

        // True when the process runs on the caller's console, so SendCtrlBreak can reach it.
        public static bool SharesConsoleWith(int pid)
        {
            var list = new uint[4096];
            uint n = GetConsoleProcessList(list, (uint)list.Length);
            if (n == 0 || n > list.Length) return false;
            for (int i = 0; i < n; i++) if (list[i] == (uint)pid) return true;
            return false;
        }

        // Ctrl+Break to the process group whose id is pid, on the caller's console.
        public static bool SendCtrlBreak(int pid) { return GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, (uint)pid); }

        // Ctrl+Break to a process group on another console: a short-lived PowerShell helper loads this file
        // (sourcePath), attaches to that console and sends the event there (SendCtrlBreakFromAttachedConsole).
        public static bool SendCtrlBreakThroughHelper(int pid, string sourcePath, int timeoutMilliseconds)
        {
            string script = "Add-Type -Path '" + sourcePath.Replace("'", "''") + "'; " +
                            "exit [ZealScripts.ConsoleProcess]::SendCtrlBreakFromAttachedConsole(" + pid + ")";
            var psi = new ProcessStartInfo(Process.GetCurrentProcess().MainModule.FileName);
            foreach (var a in new[] { "-NoProfile", "-NonInteractive", "-EncodedCommand",
                                      Convert.ToBase64String(Encoding.Unicode.GetBytes(script)) })
                psi.ArgumentList.Add(a);
            psi.UseShellExecute = false;
            psi.CreateNoWindow = true;
            using (var helper = Process.Start(psi))
            {
                if (helper.WaitForExit(timeoutMilliseconds)) return helper.ExitCode == HelperSent;
                try { helper.Kill(); } catch (InvalidOperationException) { }
                return false;
            }
        }

        // The helper's work: leave its own console, attach to the one of pid and send Ctrl+Break to pid's group while
        // ignoring it here. Returns HelperSent, HelperCannotAttach or HelperCannotSend.
        public static int SendCtrlBreakFromAttachedConsole(int pid)
        {
            FreeConsole();
            if (!AttachConsole((uint)pid)) return HelperCannotAttach;
            SetConsoleCtrlHandler(IntPtr.Zero, true);
            return GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, (uint)pid) ? HelperSent : HelperCannotSend;
        }

        static bool HasConsole()
        {
            var list = new uint[1];
            return GetConsoleProcessList(list, 1) > 0;
        }

        // name=value pairs sorted by name, each ended by a NUL, then one more NUL; null values are left out.
        static string EnvironmentBlock(IDictionary environment)
        {
            var names = new List<string>();
            foreach (DictionaryEntry e in environment) names.Add(e.Key.ToString());
            names.Sort(StringComparer.OrdinalIgnoreCase);
            var block = new StringBuilder();
            foreach (var name in names)
            {
                var value = environment[name];
                if (value == null || name.Length == 0 || name[0] == '=') continue;
                block.Append(name).Append('=').Append(value.ToString()).Append('\0');
            }
            return block.Append('\0').ToString();
        }

        // One command-line argument, quoted the way the C runtime splits a command line.
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

    // A job that ends every process in it when it closes. Only the caller holds its handle (it is never inherited),
    // so Windows closes it when the caller ends, however the caller ends.
    static class CallerJob
    {
        [StructLayout(LayoutKind.Sequential)]
        struct JOBOBJECT_BASIC_LIMIT_INFORMATION
        {
            public long PerProcessUserTimeLimit; public long PerJobUserTimeLimit; public uint LimitFlags;
            public UIntPtr MinimumWorkingSetSize; public UIntPtr MaximumWorkingSetSize; public uint ActiveProcessLimit;
            public UIntPtr Affinity; public uint PriorityClass; public uint SchedulingClass;
        }
        [StructLayout(LayoutKind.Sequential)]
        struct IO_COUNTERS { public ulong ReadOps, WriteOps, OtherOps, ReadBytes, WriteBytes, OtherBytes; }
        [StructLayout(LayoutKind.Sequential)]
        struct JOBOBJECT_EXTENDED_LIMIT_INFORMATION
        {
            public JOBOBJECT_BASIC_LIMIT_INFORMATION BasicLimitInformation; public IO_COUNTERS IoInfo;
            public UIntPtr ProcessMemoryLimit; public UIntPtr JobMemoryLimit;
            public UIntPtr PeakProcessMemoryUsed; public UIntPtr PeakJobMemoryUsed;
        }
        [StructLayout(LayoutKind.Sequential)]
        struct JOBOBJECT_BASIC_ACCOUNTING_INFORMATION
        {
            public long TotalUserTime; public long TotalKernelTime;
            public long ThisPeriodTotalUserTime; public long ThisPeriodTotalKernelTime;
            public uint TotalPageFaultCount; public uint TotalProcesses; public uint ActiveProcesses;
            public uint TotalTerminatedProcesses;
        }

        [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        static extern IntPtr CreateJobObjectW(IntPtr securityAttributes, string name);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool SetInformationJobObject(IntPtr job, int infoClass,
                                                   ref JOBOBJECT_EXTENDED_LIMIT_INFORMATION info, int size);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool QueryInformationJobObject(IntPtr job, int infoClass,
                                                     out JOBOBJECT_BASIC_ACCOUNTING_INFORMATION info, int size,
                                                     IntPtr returned);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool AssignProcessToJobObject(IntPtr job, IntPtr process);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool CloseHandle(IntPtr handle);

        const int JobObjectBasicAccountingInformation = 1;
        const int JobObjectExtendedLimitInformation = 9;
        const uint JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000;
        const int PollMilliseconds = 50;

        // IntPtr.Zero when Windows refuses the job; the programs then just aren't tied to the caller.
        static readonly IntPtr job = CreateKillOnCloseJob();

        internal static bool Add(IntPtr process)
        {
            return job != IntPtr.Zero && AssignProcessToJobObject(job, process);
        }

        // Waits until no process runs in the job any more (true), or until the time is up (false).
        internal static bool WaitUntilEmpty(int milliseconds)
        {
            if (job == IntPtr.Zero) return true;
            var clock = Stopwatch.StartNew();
            int size = Marshal.SizeOf(typeof(JOBOBJECT_BASIC_ACCOUNTING_INFORMATION));
            while (true)
            {
                JOBOBJECT_BASIC_ACCOUNTING_INFORMATION info;
                if (!QueryInformationJobObject(job, JobObjectBasicAccountingInformation, out info, size, IntPtr.Zero))
                    return false;
                if (info.ActiveProcesses == 0) return true;
                if (clock.ElapsedMilliseconds >= milliseconds) return false;
                Thread.Sleep(PollMilliseconds);
            }
        }

        static IntPtr CreateKillOnCloseJob()
        {
            IntPtr h = CreateJobObjectW(IntPtr.Zero, null);
            if (h == IntPtr.Zero) return IntPtr.Zero;
            var info = new JOBOBJECT_EXTENDED_LIMIT_INFORMATION();
            info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
            if (SetInformationJobObject(h, JobObjectExtendedLimitInformation, ref info,
                                        Marshal.SizeOf(typeof(JOBOBJECT_EXTENDED_LIMIT_INFORMATION))))
                return h;
            CloseHandle(h);
            return IntPtr.Zero;
        }
    }

    // The caller's console handler. Ctrl+C, Ctrl+Break and a closing console (or log-off, shutdown) are recorded
    // instead of ending the caller, which then stops its programs itself. After a close, log-off or shutdown event
    // Windows ends the caller soon after the handler returns, and the job then ends whatever still runs in it; the
    // programs got the same event, so the handler first gives them time to finish shutting down.
    public static class ConsoleEvents
    {
        delegate bool HandlerRoutine(uint type);
        [DllImport("kernel32.dll", SetLastError = true)]
        static extern bool SetConsoleCtrlHandler(HandlerRoutine handler, bool add);

        public const int None = -1;
        public const int CtrlC = 0;
        public const int CtrlBreak = 1;
        public const int Close = 2;
        public const int Logoff = 5;
        public const int Shutdown = 6;
        // Windows ends the process 5 s after a close, log-off or shutdown event.
        const int CloseWaitMilliseconds = 4000;

        static readonly object gate = new object();
        static readonly ManualResetEvent received = new ManualResetEvent(false);
        static HandlerRoutine handler;
        static int first = None;

        public static WaitHandle Received { get { return received; } }
        public static bool IsSet { get { return received.WaitOne(0); } }
        // The first event since Enable (CtrlC, CtrlBreak, Close, Logoff, Shutdown), or None.
        public static int FirstType { get { return Volatile.Read(ref first); } }

        public static bool Enable()
        {
            lock (gate)
            {
                if (handler != null) return true;
                handler = OnEvent;
                if (SetConsoleCtrlHandler(handler, true)) return true;
                handler = null;
                return false;
            }
        }

        public static void Disable()
        {
            lock (gate)
            {
                if (handler != null) SetConsoleCtrlHandler(handler, false);
                handler = null;
                received.Reset();
                Volatile.Write(ref first, None);
            }
        }

        static bool OnEvent(uint type)
        {
            Interlocked.CompareExchange(ref first, (int)type, None);
            received.Set();
            if (type >= Close) CallerJob.WaitUntilEmpty(CloseWaitMilliseconds);
            return true;
        }
    }
}
