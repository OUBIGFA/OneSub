param([Parameter(Mandatory=$true)][string]$Exe, [Parameter(Mandatory=$true)][string]$Work, [switch]$VideoTools, [switch]$PresetReset, [switch]$WorkbenchLayout, [switch]$ChoiceControls, [switch]$Proofreading, [switch]$Development, [switch]$ScrollFollow, [ValidateSet('dark','light')][string]$Appearance = 'dark', [string]$TranslationEndpoint, [switch]$InterruptTranslation, [switch]$ClearTranslation, [switch]$ComposeOutput, [switch]$SkipTranslation, [switch]$TaskTimings, [switch]$LengthGuide, [switch]$SubtitleIssues, [string]$PipelineModelsRoot, [string]$AsrModel='Qwen3-ASR-1.7B-hf')
$ErrorActionPreference = 'Stop'
# The driver and every child inherit a non-input desktop. Never SwitchDesktop:
# UIA and window messages exercise only the isolated application's own windows.
Add-Type @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
public static class PrivateSmoke {
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
  [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr h);
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateProcess(string app,StringBuilder command,IntPtr pa,IntPtr ta,bool inherit,uint flags,IntPtr env,string cwd,ref Startup startup,out ProcessInfo info);
  [DllImport("kernel32.dll",SetLastError=true)] static extern IntPtr CreateJobObject(IntPtr attrs,string name);
  [DllImport("kernel32.dll",SetLastError=true)] static extern bool SetInformationJobObject(IntPtr job,int type,ref JobLimits limits,int length);
  [DllImport("kernel32.dll",SetLastError=true)] static extern bool AssignProcessToJobObject(IntPtr job,IntPtr process);
  [DllImport("kernel32.dll")] static extern uint ResumeThread(IntPtr thread);
  [DllImport("kernel32.dll")] static extern uint WaitForSingleObject(IntPtr h,uint ms);
  [DllImport("kernel32.dll")] static extern bool GetExitCodeProcess(IntPtr process,out uint code);
  [DllImport("kernel32.dll")] static extern bool TerminateProcess(IntPtr process,uint code);
  [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr h);
  public static int Run(string exe,string encoded,string cwd) {
    IntPtr desktop=IntPtr.Zero,job=IntPtr.Zero; ProcessInfo p=new ProcessInfo();
    try {
      string name="OneSub-test-"+Guid.NewGuid().ToString("N");
      // Omit DESKTOP_SWITCHDESKTOP: the launcher never activates this desktop.
      desktop=CreateDesktop(name,IntPtr.Zero,IntPtr.Zero,0,0xff,IntPtr.Zero);
      if(desktop==IntPtr.Zero)throw new Win32Exception();
      job=CreateJobObject(IntPtr.Zero,null);
      if(job==IntPtr.Zero)throw new Win32Exception();
      var limits=new JobLimits();limits.basic.flags=0x2000;
      if(!SetInformationJobObject(job,9,ref limits,Marshal.SizeOf(limits)))throw new Win32Exception();
      var startup=new Startup{cb=Marshal.SizeOf(typeof(Startup)),desktop="winsta0\\"+name};
      var command=new StringBuilder("\""+exe+"\" -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand "+encoded);
      if(!CreateProcess(exe,command,IntPtr.Zero,IntPtr.Zero,false,0x08000004,IntPtr.Zero,cwd,ref startup,out p))throw new Win32Exception();
      if(!AssignProcessToJobObject(job,p.process)){TerminateProcess(p.process,1);throw new Win32Exception();}
      if(ResumeThread(p.thread)==uint.MaxValue)throw new Win32Exception();
      if(WaitForSingleObject(p.process,900000)!=0)throw new Exception("Private desktop smoke timed out");
      uint code;if(!GetExitCodeProcess(p.process,out code))throw new Win32Exception();
      return (int)code;
    } finally {
      // Kill-on-close covers only this test's descendants, including failed helpers.
      if(job!=IntPtr.Zero)CloseHandle(job);
      if(p.thread!=IntPtr.Zero)CloseHandle(p.thread);
      if(p.process!=IntPtr.Zero)CloseHandle(p.process);
      if(desktop!=IntPtr.Zero)CloseDesktop(desktop);
    }
  }
}
'@
$exePath=[IO.Path]::GetFullPath($Exe)
$workPath=[IO.Path]::GetFullPath($Work)
if(-not (Test-Path -LiteralPath $exePath)){throw 'Test executable is missing'}
if(Test-Path -LiteralPath $workPath){throw 'Use a fresh smoke directory'}
$parent=Split-Path $workPath -Parent
New-Item -ItemType Directory -Path $parent -Force | Out-Null
$log=$workPath+'.log'
function Quote-Literal([string]$value){return "'"+$value.Replace("'","''")+"'"}
$driver=Quote-Literal (Join-Path $PSScriptRoot 'native_smoke.ps1')
$capture=Quote-Literal (Join-Path $PSScriptRoot 'private_screenshot.ps1')
$command='& ([ScriptBlock]::Create([IO.File]::ReadAllText('+ $driver +',[Text.Encoding]::UTF8))) -Exe '+(Quote-Literal $exePath)+' -Work '+(Quote-Literal $workPath)+' -ScreenshotTool '+$capture+' -PrivateDesktop -OpenArgument -UIAPlayback'
if(-not $Development){$command+=' -Packaged'}
if($VideoTools){$command+=' -VideoTools'}
if($PresetReset){$command+=' -PresetReset'}
if($ChoiceControls){$command+=' -ChoiceControls'}
if($Proofreading){$command+=' -Proofreading'}
if($ScrollFollow){$command+=' -ScrollFollow'}
if($WorkbenchLayout){$command+=' -WorkbenchLayout'}
if($TranslationEndpoint){$command+=' -TranslationEndpoint '+(Quote-Literal $TranslationEndpoint)}
if($InterruptTranslation){$command+=' -InterruptTranslation'}
if($ClearTranslation){$command+=' -ClearTranslation'}
if($ComposeOutput){$command+=' -ComposeOutput'}
if($SkipTranslation){$command+=' -SkipTranslation'}
if($TaskTimings){$command+=' -TaskTimings'}
if($LengthGuide){$command+=' -LengthGuide'}
if($SubtitleIssues){$command+=' -SubtitleIssues'}
$command+=' -Appearance '+(Quote-Literal $Appearance)
if($PipelineModelsRoot){
    $bootstrap='$nativeBootstrap=[IO.File]::ReadAllText('+ $driver +',[Text.Encoding]::UTF8); & ([ScriptBlock]::Create($nativeBootstrap.Substring(0,$nativeBootstrap.IndexOf(''$workPath = '')))) -PrivateDesktop -UIAPlayback; '
    $pipeline=Quote-Literal (Join-Path $PSScriptRoot 'process_media.ps1')
    $command=$bootstrap+'& ([ScriptBlock]::Create([IO.File]::ReadAllText('+ $pipeline +',[Text.Encoding]::UTF8))) -Exe '+(Quote-Literal $exePath)+' -Work '+(Quote-Literal $workPath)+' -ScreenshotTool '+$capture+' -TranslationEndpoint '+(Quote-Literal $TranslationEndpoint)+' -ModelsRoot '+(Quote-Literal ([IO.Path]::GetFullPath($PipelineModelsRoot)))+' -AsrModel '+(Quote-Literal $AsrModel)
    if(-not $Development){$command+=' -Packaged'}
}
if($Development){
    $repoRoot=Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
    $runtimeOverrides=@{
        ONESUB_SUBTITLE_WORKER='build/output/subtitle-core/OneSub.SubtitleCore.exe'
        ONESUB_STYLE_WORKER='build/output/style-core/OneSub.StyleCore.exe'
        ONESUB_SKILL_WORKER='build/output/skill-tools/OneSub.SkillTools.exe'
        ONESUB_SKILL_ROOT='third_party/skills/srt-subtitle-translator'
        ONESUB_ONEASR_ROOT='build/output/oneasr'
        ONESUB_SHERPA_ROOT='build/output/sherpa-onnx'
    }
    $setup=''
    foreach($name in $runtimeOverrides.Keys){$setup+='$env:'+ $name +'='+ (Quote-Literal (Join-Path $repoRoot $runtimeOverrides[$name]))+'; '}
    $command=$setup+$command
}
# Transcript captures native helper output without PowerShell 5.1 treating
# normal stderr as a terminating NativeCommandError under stream redirection.
$body='$ErrorActionPreference="Stop"; $env:PSModulePath=(Join-Path $PSHOME "Modules")+";"+$env:PSModulePath; Start-Transcript -Path '+(Quote-Literal $log)+' | Out-Null; try { '+$command+'; exit 0 } catch { Write-Output $_; exit 1 } finally { Stop-Transcript | Out-Null }'
$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($body))
# The UIA driver uses Windows PowerShell's in-box desktop assemblies. Resolve
# that executable explicitly, including when the launcher is called from pwsh.
$testShell=Join-Path $env:WINDIR 'System32/WindowsPowerShell/v1.0/powershell.exe'
$code=[PrivateSmoke]::Run($testShell,$encoded,(Get-Location).Path)
Get-Content -LiteralPath $log -Encoding UTF8 | Where-Object {$_ -notmatch '^主机应用程序:|^-EncodedCommand|^Host Application:'} | Select-Object -Last 20
if($code -ne 0){throw "Private desktop smoke failed; see $log"}
