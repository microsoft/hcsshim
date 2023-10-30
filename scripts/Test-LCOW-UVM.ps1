#ex: .\scripts\Test-LCOW-UVM.ps1 -vb -Count 2 -Benchtime '3s'
# benchstat via  `go install golang.org/x/perf/cmd/benchstat@latest`

[CmdletBinding()]
param (
    [ValidateSet('Test', 'Bench', 'List', 'Shell')]
    [alias('a')]
    [string]
    $Action = 'Bench',

    [string]
    $Note = '',

    # test parameters
    [switch]
    $Shuffle,

    [int]
    $Count = 1,

    [string]
    $BenchTime = '5s',

    [string]
    $Timeout = '10m',

    [alias('tv')]
    [switch]
    $TestVerbose,

    [string]
    $Run,

    [string]
    $OutDirectory = '.\test\results',

    # uvm parameters

    [string]
    $UVMBootPath = '.\bin\cmd\uvmboot.exe',

    [string]
    $BootFilesPath = 'C:\ContainerPlat\LinuxBootFiles',

    [ValidateSet('vhd', 'initrd')]
    [string]
    $BootFSType = 'vhd',

    [switch]
    $DisableTimeSync,

    # gcs test/container options

    # we can no longer specify the guest/destination path for SCSI mounts, so hope the rootfs is the first
    # path to be SCSI-mounted
    [string]
    $ContainerRootFSMount = '/run/mounts/scsi/m0',

    [string]
    $ContainerRootFSPath = (Join-Path $BootFilesPath 'rootfs.vhd'),

    [string]
    $GCSTestMount = '/run/bin',

    [string]
    $GCSTestPath = '.\bin\test\gcs.test',

    [switch]
    $SkipGCSTestMount,

    [string[]]
    $Features
)
$ErrorActionPreference = 'Stop'
Import-Module ( Join-Path $PSScriptRoot Testing.psm1 ) -Force

$BootFilesPath = Resolve-Path $BootFilesPath
$ContainerRootFSPath = Resolve-Path $ContainerRootFSPath
$GCSTestPath = Resolve-Path $GCSTestPath
$UVMBootPath = Resolve-Path $UVMBootPath

$shell = ( $Action -eq 'Shell' )

$date = Get-Date
if ( $shell ) {
    $cmd = 'ash'
} else {
    $waitfiles = "$ContainerRootFSMount"
    $gcspath = 'gcs.test'
    if ( -not $SkipGCSTestMount ) {
        $waitfiles += ",$GCSTestMount"
        $gcspath = "$GCSTestMount/gcs.test"
    }

    $pre = "wait-paths -p $waitfiles -t 5 ; "

    $testcmd, $out = New-TestCommand `
        -Action $Action `
        -Path $gcspath `
        -Name gcstest `
        -OutDirectory $OutDirectory `
        -Date $date `
        -Note $Note `
        -Shuffle:$Shuffle `
        -TestVerbose:$TestVerbose `
        -Count $Count `
        -BenchTime $BenchTime `
        -Timeout $Timeout `
        -Run $Run `
        -Features $Features `
        -Verbose:$Verbose

    $testcmd += " `'-rootfs-path=$ContainerRootFSMount`' "
    $cmd = $pre + $testcmd
}

$boot = "$UVMBootPath -gcs lcow " + `
    '-fwd-stdout -fwd-stderr -output-handling stdout ' + `
    "-boot-files-path $BootFilesPath " + `
    "-root-fs-type $BootFSType " + `
    '-kernel-file vmlinux ' + `
    "-mount-scsi `"$ContainerRootFSPath`" "

if ( -not $SkipGCSTestMount ) {
    $boot += "-share `"$GCSTestPath,$GCSTestMount`" "
}

if ( $DisableTimeSync ) {
    $boot += ' -disable-time-sync '
}

if ( $shell ) {
    $boot += ' -t '
}

$boot += " -exec `"$cmd`" "

Invoke-TestCommand `
    -TestCmd $boot `
    -OutputFile (& { if ( $Action -ne 'Shell' ) { $out } }) `
    -OutputCmd (& { if ( $Action -eq 'Bench' ) { 'benchstat' } }) `
    -Verbose:$Verbose
