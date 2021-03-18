$ErrorActionPreference = 'Stop'

$packageName = 'kubectl-buildkit'

$toolsPath = Split-Path $MyInvocation.MyCommand.Definition

$packageArgs = @{
  PackageName    = $packageName
  FileFullPath64 = Get-Item $toolsPath\*.tar.gz
  Destination    = $toolsPath
}
Get-ChocolateyUnzip @packageArgs

if (Test-Path "$toolsPath\kubectl-buildkit*.tar") {
  $packageArgs2 = @{
    PackageName    = $packageName
    FileFullPath64 = Get-Item $toolsPath\*.tar
    Destination    = $toolsPath
  }
  Get-ChocolateyUnzip @packageArgs2

  Remove-Item "$toolsPath\kubectl-buildkit*.tar"
}