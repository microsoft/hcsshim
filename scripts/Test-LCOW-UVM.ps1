#ex: .\scripts\Test-LCOW-UVM.ps1 -vb -Action Bench -BootFilesPath C:\ContainerPlat\LinuxBootFiles\ -MountGCSTest -Count 2 -Benchtime '3s'
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
    $Run = '',

    [string]
    $CodePath = '.',

    [string]
    $OutDirectory = '.\test\results',

    # uvm parameters

    [string]
    $BootFilesPath = 'C:\ContainerPlat\LinuxBootFiles',

    [ValidateSet('vhd', 'initrd')]
    [string]
    $BootFSType = 'vhd',

    [switch]
    $DisableTimeSync,

    # gcs test/container options

    [string]
    $ContainerRootFSMount = '/run/rootfs',

    [string]
    $ContainerRootFSPath = (Join-Path $BootFilesPath 'rootfs.vhd'),

    [string]
    $GCSTestMount = '/run/bin',

    [string]
    $GCSTestPath = '.\bin\test\gcs.test',

    [switch]
    $MountGCSTest,

    [string]
    $Feature = ''
)

Import-Module ( Join-Path $PSScriptRoot Testing.psm1 ) -Force

$CodePath = Resolve-Path $CodePath
$OutDirectory = Resolve-Path $OutDirectory
$BootFilesPath = Resolve-Path $BootFilesPath
$ContainerRootFSPath = Resolve-Path $ContainerRootFSPath
$GCSTestPath = Resolve-Path $GCSTestPath

$shell = ( $Action -eq 'Shell' )

if ( $shell ) {
    $cmd = 'ash'
} else {
    $date = Get-Date
    $waitfiles = "$ContainerRootFSMount"
    $gcspath = 'gcs.test'
    if ( $MountGCSTest ) {
        $waitfiles += ",$GCSTestMount"
        $gcspath = "$GCSTestMount/gcs.test"
    }

    $pre = "wait-paths -p $waitfiles -t 5 ; " + `
        'echo nproc: `$(nproc) ; ' + `
        'echo kernel: `$(uname -a) ; ' + `
        'echo gcs.commit: `$(cat /info/gcs.commit 2>/dev/null) ; ' + `
        'echo gcs.branch: `$(cat /info/gcs.branch 2>/dev/null) ; ' + `
        'echo tar.date:   `$(cat /info/tar.date 2>/dev/null) ; ' + `
        'echo image.name: `$(cat /info/image.name 2>/dev/null) ; ' + `
        'echo build.date: `$(cat /info/build.date 2>/dev/null) ; '

    $testcmd, $out = New-TestCommand `
        -Action $Action `
        -Path $gcspath `
        -Name gcstest `
        -OutDirectory $OutDirectory `
        -Date $date `
        -Note $Note `
        -TestVerbose:$TestVerbose `
        -Count $Count `
        -BenchTime $BenchTime `
        -Timeout $Timeout `
        -Run $Run `
        -Feature $Feature `
        -Verbose:$Verbose

    $testcmd += " `'-rootfs-path=$ContainerRootFSMount`' "
    $cmd = $pre + $testcmd
}

$boot = '.\bin\tool\uvmboot.exe -gcs lcow ' + `
    '-fwd-stdout -fwd-stderr -output-handling stdout ' + `
    "-boot-files-path $BootFilesPath " + `
    "-root-fs-type $BootFSType " + `
    '-kernel-file vmlinux ' + `
    "-mount-scsi `"$ContainerRootFSPath,$ContainerRootFSMount`" "

if ( $MountGCSTest ) {
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
    -TestCmdPreamble $testcmd `
    -OutputFile (&{ if ( $Action -ne 'Shell' ) { $out } }) `
    -OutputCmd (&{ if ( $Action -eq 'Bench' ) { 'benchstat' } }) `
    -Preamble `
    -Date $Date `
    -Note $Note `
    -Verbose:$Verbose
