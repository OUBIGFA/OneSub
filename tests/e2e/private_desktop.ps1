if (-not ('PrivateSmoke' -as [type])) {
Add-Type @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
public static class PrivateSmoke {
  [StructLayout(LayoutKind.Sequential)] struct SecurityAttributes { public int length; public IntPtr descriptor; public int inherit; }
  [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)] struct Startup {
    public int cb; public string reserved,desktop,title;
    public int x,y,w,h,xchars,ychars,fill,flags; public short show,reserved2;
    public IntPtr reservedPtr,input,output,error;
  }
  [StructLayout(LayoutKind.Sequential)] struct ProcessInfo { public IntPtr process,thread; public int pid,tid; }
  [StructLayout(LayoutKind.Sequential)] struct BasicLimits {
    public long processTime,jobTime; public uint flags; public UIntPtr min,max;
    public uint count; public UIntPtr affinity; public uint priority,scheduling;
  }
  [StructLayout(LayoutKind.Sequential)] struct JobLimits {
    public BasicLimits basic;
    public ulong readOps,writeOps,otherOps,readBytes,writeBytes,otherBytes;
    public UIntPtr processMemory,jobMemory,peakProcess,peakJob;
  }
  [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr CreateDesktop(string name,IntPtr device,IntPtr mode,uint flags,uint access,IntPtr security);
  [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr handle);
  [DllImport("user32.dll",SetLastError=true)] static extern IntPtr OpenInputDesktop(uint flags,bool inherit,uint access);
  [DllImport("user32.dll",SetLastError=true)] static extern IntPtr GetThreadDesktop(uint thread);
  [DllImport("user32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool GetUserObjectInformation(IntPtr handle,int index,StringBuilder value,int length,out int needed);
  [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern IntPtr CreateFile(string name,uint access,uint share,ref SecurityAttributes security,uint creation,uint attributes,IntPtr template);
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcess(string app,StringBuilder command,IntPtr pa,IntPtr ta,bool inherit,uint flags,IntPtr env,string cwd,ref Startup startup,out ProcessInfo info);
  [DllImport("kernel32.dll",SetLastError=true)] static extern IntPtr CreateJobObject(IntPtr attrs,string name);
  [DllImport("kernel32.dll",SetLastError=true)] static extern bool SetInformationJobObject(IntPtr job,int type,ref JobLimits limits,int length);
  [DllImport("kernel32.dll",SetLastError=true)] static extern bool AssignProcessToJobObject(IntPtr job,IntPtr process);
  [DllImport("kernel32.dll")] static extern uint ResumeThread(IntPtr thread);
  [DllImport("kernel32.dll")] static extern uint WaitForSingleObject(IntPtr handle,uint ms);
  [DllImport("kernel32.dll")] static extern bool GetExitCodeProcess(IntPtr process,out uint code);
  [DllImport("kernel32.dll")] static extern bool TerminateProcess(IntPtr process,uint code);
  [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
  static string DesktopName(IntPtr handle) {
    if(handle==IntPtr.Zero)throw new Win32Exception();
    var name=new StringBuilder(256);int needed;
    if(!GetUserObjectInformation(handle,2,name,name.Capacity*2,out needed))throw new Win32Exception();
    return name.ToString();
  }
  public static string CurrentDesktopName() { return DesktopName(GetThreadDesktop(GetCurrentThreadId())); }
  public static bool IsPrivateDesktop() {
    string name=CurrentDesktopName();
    if(!name.StartsWith("OneSub-test-",StringComparison.Ordinal))return false;
    IntPtr input=OpenInputDesktop(0,false,1);
    try { return !String.Equals(name,DesktopName(input),StringComparison.OrdinalIgnoreCase); }
    finally { if(input!=IntPtr.Zero)CloseDesktop(input); }
  }
  public static int Run(string exe,string encoded,string cwd) { return Run(exe,encoded,cwd,900000); }
  public static int Run(string exe,string encoded,string cwd,uint timeout) {
    return RunProcess(exe,"-EncodedCommand "+encoded,cwd,null,timeout);
  }
  public static int RunFile(string exe,string script,string cwd,string log,uint timeout) {
    return RunProcess(exe,"-File \""+script+"\"",cwd,log,timeout);
  }
  static int RunProcess(string exe,string arguments,string cwd,string log,uint timeout) {
    IntPtr desktop=IntPtr.Zero,job=IntPtr.Zero,output=IntPtr.Zero,input=IntPtr.Zero; ProcessInfo process=new ProcessInfo();
    try {
      string name="OneSub-test-"+Guid.NewGuid().ToString("N");
      desktop=CreateDesktop(name,IntPtr.Zero,IntPtr.Zero,0,0xff,IntPtr.Zero);
      if(desktop==IntPtr.Zero)throw new Win32Exception();
      job=CreateJobObject(IntPtr.Zero,null);
      if(job==IntPtr.Zero)throw new Win32Exception();
      var limits=new JobLimits();limits.basic.flags=0x2000;
      if(!SetInformationJobObject(job,9,ref limits,Marshal.SizeOf(limits)))throw new Win32Exception();
      var startup=new Startup{cb=Marshal.SizeOf(typeof(Startup)),desktop="winsta0\\"+name};
      if(log!=null) {
        var security=new SecurityAttributes{length=Marshal.SizeOf(typeof(SecurityAttributes)),inherit=1};
        output=CreateFile(log,0x40000000,1,ref security,1,0x80,IntPtr.Zero);
        if(output==new IntPtr(-1)){output=IntPtr.Zero;throw new Win32Exception();}
        input=CreateFile("NUL",0x80000000,3,ref security,3,0x80,IntPtr.Zero);
        if(input==new IntPtr(-1)){input=IntPtr.Zero;throw new Win32Exception();}
        startup.flags=0x100;startup.input=input;startup.output=output;startup.error=output;
      }
      var command=new StringBuilder("\""+exe+"\" -NoProfile -NonInteractive -OutputFormat Text -ExecutionPolicy Bypass "+arguments);
      if(!CreateProcess(exe,command,IntPtr.Zero,IntPtr.Zero,log!=null,0x08000004,IntPtr.Zero,cwd,ref startup,out process))throw new Win32Exception();
      if(!AssignProcessToJobObject(job,process.process)){TerminateProcess(process.process,1);throw new Win32Exception();}
      if(ResumeThread(process.thread)==uint.MaxValue)throw new Win32Exception();
      if(WaitForSingleObject(process.process,timeout)!=0)throw new TimeoutException("Private desktop test timed out");
      uint code;if(!GetExitCodeProcess(process.process,out code))throw new Win32Exception();
      return (int)code;
    } finally {
      if(job!=IntPtr.Zero)CloseHandle(job);
      if(process.process!=IntPtr.Zero)WaitForSingleObject(process.process,5000);
      if(process.thread!=IntPtr.Zero)CloseHandle(process.thread);
      if(process.process!=IntPtr.Zero)CloseHandle(process.process);
      if(input!=IntPtr.Zero)CloseHandle(input);
      if(output!=IntPtr.Zero)CloseHandle(output);
      if(desktop!=IntPtr.Zero)CloseDesktop(desktop);
    }
  }
}
'@
}

function Invoke-OneSubPrivateDesktop {
    param(
        [Parameter(Mandatory=$true)][string]$Command,
        [Parameter(Mandatory=$true)][string]$Log,
        [Parameter(Mandatory=$true)][string]$WorkingDirectory,
        [ValidateRange(1,7200)][int]$TimeoutSeconds = 900
    )
    $logPath = [IO.Path]::GetFullPath($Log)
    if (Test-Path -LiteralPath $logPath) { throw 'Use a fresh private-desktop log path' }
    New-Item -ItemType Directory -Path (Split-Path $logPath -Parent) -Force | Out-Null
    $script = '$ErrorActionPreference=''Stop''; $OutputEncoding=[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); ' + $Command + '; if ($LASTEXITCODE) { exit $LASTEXITCODE }'
    $scriptPath = $logPath + '.ps1'
    if (Test-Path -LiteralPath $scriptPath) { throw 'Use a fresh private-desktop script path' }
    [IO.File]::WriteAllText($scriptPath, $script, [Text.UTF8Encoding]::new($true))
    return [PrivateSmoke]::RunFile((Get-Command powershell.exe).Source, $scriptPath, [IO.Path]::GetFullPath($WorkingDirectory), $logPath, [uint32]($TimeoutSeconds * 1000))
}
