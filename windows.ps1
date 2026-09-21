$VSN = "1.4.10"

# both binaries are installed side by side in the same directory
$InstallDir = "$env:LOCALAPPDATA\mmdbctl"

foreach ($Bin in "mmdbctl", "mmdbshrink") {
  # build the filename for the Zip archive and exe file
  $FileName = "$($Bin)_$($VSN)_windows_amd64"
  $ZipFileName = "$($FileName).zip"

  # download and extract zip
  Invoke-WebRequest -Uri "https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-$VSN/$FileName.zip" -OutFile ./$ZipFileName
  Unblock-File ./$ZipFileName
  Expand-Archive -Path ./$ZipFileName  -DestinationPath $InstallDir -Force

  # delete if already exists
  if (Test-Path "$InstallDir\$Bin.exe") {
    Remove-Item "$InstallDir\$Bin.exe"
  }
  Rename-Item -Path "$InstallDir\$FileName.exe" -NewName "$Bin.exe"

  # cleaning files
  Remove-Item -Path ./$ZipFileName
}

# setting up env.
$PathContent = [Environment]::GetEnvironmentVariable('path', 'Machine')

# if Path already exists
if ($PathContent -ne $null) {
  if (-Not($PathContent -split ';' -contains $InstallDir)) {
    [System.Environment]::SetEnvironmentVariable("PATH", $Env:Path + ";$InstallDir", "Machine")
  }
}
else {
  [System.Environment]::SetEnvironmentVariable("PATH", $Env:Path + ";$InstallDir", "Machine")
}

"You can use mmdbctl and mmdbshrink now."
