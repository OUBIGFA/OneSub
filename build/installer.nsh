; Present Microsoft's original terms before writing application files.
; Silent installation requires explicit acceptance of the DXC terms.
!include "FileFunc.nsh"
!define ONESUB_DXC_LICENSE "${__FILEDIR__}\..\third_party\licenses\dxc\LICENSE-MS.txt"
!macro customPages
  !define MUI_LICENSEPAGE_CHECKBOX
  !insertmacro MUI_PAGE_LICENSE "${ONESUB_DXC_LICENSE}"
!macroend

!macro customInit
  IfSilent 0 dxcAccepted
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/ACCEPT_DXC_LICENSE" $R1
  IfErrors 0 dxcAccepted
  SetErrorLevel 2
  Abort
  dxcAccepted:
!macroend
