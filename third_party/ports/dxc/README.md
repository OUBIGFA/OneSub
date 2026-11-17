# Microsoft DirectX Shader Compiler

OneAsr's separate MIT-licensed process uses the unmodified Microsoft DXC
redistributables for Direct3D 12. OneSub does not link DXC into its AGPL program
or apply its own license to Microsoft's binaries. The Windows-only components
retain their original terms; see `licenses/dxc/` in the installation.

The exact NuGet package, x64 DLLs and original notices are registered in
`third_party/manifests/dxc.json`. `tools/prepare_dxc.py` checks the archive and
each member before placing the DLLs beside `oneasr-cli.exe`. A verified cache
is reused. Runtime execution and redistribution remain subject to the original
Microsoft, LLVM and MIT terms; the installation presents those terms separately.

The package is Microsoft.Direct3D.DXC 1.9.2609.5 from Microsoft's NuGet account:
https://www.nuget.org/packages/Microsoft.Direct3D.DXC/1.9.2609.5
