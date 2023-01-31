# ex: .\scripts\Run-Tests.ps1 -vb -TestExe Functional -Count 2 -BenchTime "2x"

[CmdletBinding()]
param (
    [ValidateSet('Test', 'Bench', 'List')]
    [alias('a')]
    [string]
    $Action = 'Bench',

    [string]
    $Note,

    [string]
    $OutDirectory = '.\test\results',

    [string]
    $TestDirectory = '.\bin\test',

    [ValidateSet('Functional', 'CRI')]
    [string]
    $TestExe = 'CRI',

    [string]
    $BenchstatPath = 'benchstat.exe',

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

    [string[]]
    $Features
)
$ErrorActionPreference = 'Stop'
Import-Module ( Join-Path $PSScriptRoot Testing.psm1 ) -Force

$exe = Switch ($TestExe) {
    'Functional' { 'functional.test.exe'; break }
    'CRI' { 'cri-containerd.test.exe'; break }
}
$test = Join-Path $TestDirectory $exe

$date = Get-Date
$testcmd, $out = New-TestCommand `
    -Action $Action `
    -Path $test `
    -Name ($exe -replace '.test.exe$', '') `
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

Invoke-TestCommand `
    -TestCmd $testcmd `
    -OutputFile $out `
    -OutputCmd (& { if ( $Action -eq 'Bench' ) { $BenchstatPath } }) `
    -Preamble `
    -Date $Date `
    -Note $Note `
    -Verbose:$Verbose
