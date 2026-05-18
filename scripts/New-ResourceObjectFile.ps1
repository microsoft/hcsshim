#Requires -Version 7

<#
.SYNOPSIS

Generate *.syso resource files to be linked into Go binaries.

.DESCRIPTION

Use RC.exe and then CVTRES.exe to embed the manifest as well as other resource information
into a COFF object file that the go compiler can automatically link into the final binary.
While similar to what github.com/josephspurrier/goversioninfo/cmd/goversioninfo does
(and is intended to also be used in go:generate directives), the resulting object file is
different in structure than the `goversioninfo` output.
Specifically, it has multiple sections (.debug$S, .rsrc$01, and .rsrc$03), and causes `go build`
to fail with "sectnum < 0!" unless the `-ldflags=-linkmode=external` flag is used,
which requires enabling CGO.

.EXAMPLE

New-ResourceObjectFile -Destination .\cmd\runhcs -Name runhcs -Version 0.14.0 -Architecture all

Create resource_windows_amd64.syso and resource_windows_arm64.syso files in the cmd\runhcs directory.

.LINK

https://learn.microsoft.com/en-us/windows/win32/sbscs/application-manifests

.LINK

https://learn.microsoft.com/en-us/windows/win32/menurc/about-resource-files

#>
[CmdletBinding()]
param (
    [ValidateNotNullOrEmpty()]
    [ValidateScript( {
            return Test-Path -PathType 'Container' -Path $_ -ErrorAction 'Ignore'
        },
        ErrorMessage = "Invalid or non-existant path: '{0}'.")]
    [Parameter(Mandatory = $true)]
    [string]
    $Destination,

    # Application name.
    [ValidateNotNullOrEmpty()]
    [Parameter(Mandatory = $true)]
    [string]
    $Name,

    # Application version.
    [ValidateNotNullOrEmpty()]
    [version]
    $Version = '1.0.0.0',

    # Read version from internal\version\data\VERSION.
    # Falls back on $Version parameter if file is not found.
    #
    # See internal\version\version.go or Set-VersionInfo.ps1 for more info.
    [ValidateNotNullOrEmpty()]
    [switch]
    $UseVersionFile,

    # Processors architecture to target.
    # Use all to generate for all supported architectures.
    [ValidateSet('amd64', 'arm64', 'all')]
    $Architecture = 'amd64',

    # Visual Studio installation path
    [ValidateNotNullOrEmpty()]
    [ValidateScript( {
            return Test-Path -PathType 'Container' -Path $_ -ErrorAction 'Ignore'
        },
        ErrorMessage = "Invalid or non-existant directory: '{0}'.")]
    [string]
    $VSPath
)

$root = Split-Path $PSScriptRoot

function run([string]$cmd) {
    Write-Verbose "Running command: $cmd"
    $r = & ${env:COMSPEC} /C $cmd 2>&1
    if ( $LASTEXITCODE -ne 0 ) {
        Write-Host $r
        Write-Error -ErrorAction 'Stop' "Command failed: $cmd"
    }
    $r
}

function Get-PackageVersion {
    [CmdletBinding()]
    param (
        [string]
        $file = 'internal\version\data\VERSION'
    )
    Write-Verbose "Reading version from version file: $file"

    $vf = Join-Path -Resolve $root $file -ErrorAction 'Ignore'
    if ( [string]::IsNullOrEmpty($vf) ) {
        Write-Error "Version file $file does not exist" -ErrorAction 'Continue'
        return
    }

    $s = (Get-Content $vf -ErrorAction 'Ignore').Trim()
    if ( [string]::IsNullOrWhiteSpace($s) ) {
        Write-Error "Empty version file: ${vf}" -ErrorAction 'Continue'
        return
    }

    # remove "v" prefix
    $s = $s.Trim() -replace '^v', ''
    Write-Verbose "Read version from file `"${vf}`": $s"
    try {
        $v = [semver]$s
    } catch {
        Write-Error "Invalid version string: $s" -ErrorAction 'Continue'
        return
    }
    [version]::new($v.Major, $v.Minor, $v.Patch, 0)
}

$Destination = Resolve-Path $Destination -ErrorAction 'Stop'

$manifestDir = New-Item -ItemType 'Directory' -Force -Path (Join-Path $env:TEMP 'manifest' "$Name-$(New-Guid)") -ErrorAction 'Stop'
$Manifest = Join-Path $manifestDir 'manifest.xml' -ErrorAction 'Stop'

if ( $UseVersionFile ) {
    $v = Get-PackageVersion -ErrorAction 'Continue'
    if ( $null -ne $v ) {
        # Fall back on default version if file is empty
        $Version = $v
    }
    Write-Verbose "Using version: $Version"
}

[xml]$m = @'
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
    <assemblyIdentity type="win32" />
    <asmv3:application>
        <!-- https://learn.microsoft.com/en-us/windows/win32/fileio/maximum-file-path-limitation -->
        <asmv3:windowsSettings xmlns:ws2="http://schemas.microsoft.com/SMI/2016/WindowsSettings">
            <ws2:longPathAware>true</ws2:longPathAware>
        </asmv3:windowsSettings>
    </asmv3:application>
    <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
        <application>
            <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}" />
        </application>
    </compatibility>
</assembly>
'@

$m.assembly.assemblyIdentity.SetAttribute('name', "Microsoft.ContainerPlatform.$Name")
$m.assembly.assemblyIdentity.SetAttribute('version', $Version)
if ( $Architecture -ne 'all' ) {
    # valid values: x86, amd64, arm, arm64
    $m.assembly.assemblyIdentity.SetAttribute('processorArchitecture', $Architecture.ToLower())
}

$m.PreserveWhitespace = $false
$m.Save($Manifest)
Write-Verbose "Wrote manifest file: $Manifest"

if ( [string]::IsNullOrEmpty($VSPath) ) {
    $vswhere = Join-Path -Resolve ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe' -ErrorAction 'Stop'
    $VSPath = run "`"$vswhere`" -prerelease -latest -property installationPath"
} else {
    $VSPath = Resolve-Path $VSPath -ErrorAction 'Stop'
}
Write-Verbose "Using Visual Studio installation: $VSPath"

$vsdevbat = Join-Path -Resolve $VSPath '\Common7\Tools\vsdevcmd.bat' -ErrorAction 'Ignore'
if ( [string]::IsNullOrEmpty($vsdevbat) ) {
    Write-Error -ErrorAction 'Stop' "Missing 'vsdevcmd.bat' tool in valid Visual Studio SDK installation: $VSPath"
}
Write-Verbose "Visual Studio CMD batch file: $vsdevbat"
# surpress output line of "Parsing of manifest successful."
$null = run "cd /d ${manifestDir}&&`"${vsdevbat}`" -no_logo&&mt.exe -nologo -manifest $(Split-Path -Leaf $manifest) -validate_manifest"

$rc = Join-Path $manifestDir 'resource.rc' -ErrorAction 'Stop'
$vstr = "$($Version.Major), $($Version.Minor), $($Version.Build), 0"

# TODO: set VER_PRODUCT* fields with package release info
@"
#include <windows.h>

#pragma code_page(65001) // UTF-8

#define VER_FILEDESCRIPTION_STR "Microsoft Container Platform $Name"
#define VER_FILEVERSION         $vstr
#define VER_FILEVERSION_STR     "$version"

#define VER_PRODUCTNAME_STR     "Microsoft Container Platform"
#define VER_PRODUCTVERSION      $vstr
#define VER_PRODUCTVERSION_STR  "$version"

#define VER_INTERNALNAME_STR        "${Name}.exe"
#define VER_ORIGINALFILENAME_STR    "${Name}.exe"

#define VER_COMPANYNAME_STR "Microsoft Corporation"

#define VER_FILEFLAGSMASK   VS_FFI_FILEFLAGSMASK
#define VER_FILEFLAGS       0
#define VER_FILEOS          VOS_NT_WINDOWS32
#define VER_FILETYPE        VFT_APP
#define VER_FILESUBTYPE     VFT2_UNKNOWN

#define MANIFEST_RESOURCE_ID 1
MANIFEST_RESOURCE_ID RT_MANIFEST "manifest.xml"

#include <common.ver>
"@ | Out-File -Encoding 'ascii' -FilePath $rc -ErrorAction 'Stop'

run "cd /d ${manifestDir}&&`"${vsdevbat}`" -no_logo&&rc.exe /nologo $(Split-Path -Leaf $rc)"

# Conveniently, the accepted processorArchitecture values match GOARCH values,
# but CVTRES needs its own constants (ARM, ARM64, ARM64EC, ARM64X, EBC, IA64, X64, X86)
foreach ( $p in @{
        'amd64' = 'X64'
        'arm64' = 'ARM64'
    }.GetEnumerator() ) {
    $arch = $p.Key
    if ( ($Architecture -ne $arch) -and ($Architecture -ne 'all') ) {
        continue
    }

    $syso = Join-Path $Destination "resource_windows_${arch}.syso"
    Write-Verbose "Generating $arch resource file: $syso"
    run "cd /d ${manifestDir}&&`"${vsdevbat}`" -no_logo&&CVTRES.exe /NOLOGO /MACHINE:$($p.Value) `"/OUT:${syso}`" resource.res"
}
