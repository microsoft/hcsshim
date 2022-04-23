[CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
param (
    [string]
    # The path to the build file. Do not include `~` in the path
    $Path = '.\build.ninja',

    [string]
    # The path to the root of the code directory
    $Root = '.',

    [string]
    $PwshSource = '',

    [string[]]
    $PwshFlags = '',

    [string]
    $GoSource = '',

    [string[]]
    $GoBuildFlags = '',

    [string[]]
    $GoTestFlags = '',

    [switch]
    $NoCrictl,

    [string]
    $CrictlSource = '',

    [string[]]
    $CrictlFlags = ("--image-endpoint 'tcp://127.0.0.1:2376'", "--runtime-endpoint 'tcp://127.0.0.1:2376'"),

    [string]
    $CPlatPath = 'C:\ContainerPlat',

    [string]
    $CPlatDataPath = 'C:\ContainerPlatData',

    [switch]
    $NoProto,

    [string]
    $ProtobuildSource = '',

    [string]
    # the version of protobuild to download in the ninja file
    $ProtobuildVersion = '0.2.0',

    [string[]]
    $ProtobuildFlags = '',

    [string]
    $ProtocSource = '',

    [string]
    # the version of protoc to download in the ninja file
    $ProtocVersion = '3.19.4',

    [string]
    $ProtocCtrdSource = '',

    [switch]
    $Info
)

# todo: `Install-Module -Name PSScriptAnalyzer` and run on .\scripts\*
# todo: split up into sub-ninjas for misc and proto

# todo: make "command/rule" object that takes source/dest/name and writes build declaration
# todo: abstract away using SourceVar/DestVar vs $in/$out

# ninja strips leading `.` and trailing slashes from relative
# paths for $in and $out, but not for other variables.
# So, for now, build targets and dependencies will all be relative to package root
# todo: absolute paths everythwhere -  https://github.com/ninja-build/ninja/issues/1251

####################################################################################################
# setup
####################################################################################################

Import-Module ( Join-Path $PSScriptRoot NinjaBuild ) -Force -Verbose:$false

Write-Verbose 'Resolving paths and commands'

$Root = Resolve-PathError $Root
Write-Verbose "Using root path `"$Root`""

if ( -not (Test-Path $Path -IsValid) ) {
    throw "`"$Path`" is not a valid path"
}
if ( -not ( [System.IO.Path]::IsPathRooted($Path) ) ) {
    $Path = Join-Path $Root $Path
}
$Path = [IO.Path]::GetFullPath($Path)
Write-Verbose "Using build file path `"$Path`""

$GoModule = Get-GoModuleName -Path '.'
if ( -not $GoModule ) {
    throw "Could not retreive the go module name in $(Get-Item '.')"
}
Write-Verbose "Found module name `"$GoModule`""

$GoPath = Get-GoEnv 'GOPATH'
Write-Verbose "Using GOPATH `"$GoPath`""

$Bin = Join-Path $Root 'bin'
$CmdsBin = "$Bin\cmd"
$ToolsBin = "$Bin\tools"
$TestsBin = "$Bin\test"
$OutDir = Join-Path $Root 'out'
$DepsDir = Join-Path $Root 'deps'
$dirs = [string[]]($CmdsBin, $ToolsBin, $TestsBin, $OutDir, $DepsDir)

$PwshSource = Resolve-Command -Name 'pwsh.exe' -Path $PwshSource
Write-Verbose "Using powershell `"$PwshSource`""

$GoSource = Resolve-Command -Name 'go.exe' -Path $GoSource
Write-Verbose "Using go `"$GoSource`""

$AllDeps = [string[]]''

####################################################################################################
# move setup
####################################################################################################

$moverules = $True

try {
    $CPlatPath = Resolve-PathError -Path $CPlatPath -Name 'Container Platform installation directory'
    Write-Verbose "Using Container Platform installation directory `"$CPlatPath`""
    $CPlatDataPath = Resolve-PathError $CPlatDataPath -Name 'Container Platform data directory'
    Write-Verbose "Using Container Platform data directory `"$CPlatDataPath`""
} catch {
    Write-Warning "$_ Skipping move rules."
    $moverules = $False
}

####################################################################################################
# proto setup
####################################################################################################

$ProtoDir = Join-Path $Root 'protobuf'
$ProtoBin = Join-Path $Root 'bin\protobuf'

$ProtocZip = "$OutDir\protoc.zip"
$LocalProtocSource = "$ProtoBin\protoc.exe"
$LocalProtobuildSource = "$GoPath\bin\protobuild.exe"

$installprotoc = $False
$installprotobuild = $False

if ( -not $NoProto ) {
    foreach ( $p in ( @{
                N = 'protobuild.exe'; S = ([ref]$ProtobuildSource); V = $ProtobuildVersion;
                I = ([ref]$installprotobuild) ; L = $LocalProtobuildSource
            },
            @{
                N = 'protoc.exe'; S = ([ref]$ProtocSource); V = $ProtocVersion;
                I = ([ref]$installprotoc) ; L = $LocalProtocSource
            }) ) {
        try {
            $v = Resolve-Command -Name $p['N'] -Path $p['S'].Value
            $p['S'].Value = ([string]$v)
            Write-Verbose "Using $($p['N']) `"$($p['S'].Value)`""
        } catch {
            $v = $p['V']
            if ( -not $v ) {
                throw "$($p['N']) version cannot be empty if source (`"$($p['S'].Value)`") is invalid"
            }
            Write-Warning "$_ Adding rules to install `"$($p['N'])`" (v$v)."
            # force the source to the local installation location, and flag it for installation
            $p['S'].Value = ([string]$p['L'])
            $p['I'].Value = $True
        }
    }

    # todo: when "go install github.com/containerd/containerd/cmd/protoc-gen-gogoctrds@latest" works, move this up
    try {
        $ProtocCtrdSource = Resolve-Command -Name 'protoc-gen-gogoctrd' -Path $ProtocCtrdSource
        Write-Verbose "Using protoc-gen-gogoctrd `"$ProtocCtrdSource`""
    } catch {
        Write-Error "$_ (Its a pain to install automatically.) Skipping protobuf rules."
        $NoProto = $True
    }
}

$dirs += $ProtoDir
if ( -not $NoProto ) {
    $dirs += $ProtoBin
}

####################################################################################################
# file preamble
####################################################################################################

New-NinjaBuildFile -Path $Path -GoModule $GoModule -Quiet

$PwshCmd = Add-PwshRule -Path $Path -Source $PwshSource `
    -Flags (('-NoProfile', '-NoLogo', '-NonInteractive') + $PwshFlags)
Add-NewLine -Path $Path -Quiet

Add-Self -Path $Path -PwshCmd $PwshCmd -Invocation $MyInvocation -Location (Get-Location)
Add-NewLine -Path $Path -Quiet

$misc = Add-MiscRule -Path $Path -PwshCmd $PwshCmd
Add-NewLine -Path $Path -Quiet

####################################################################################################
# installation directories
####################################################################################################

$dirs = $dirs | Get-Unique
$rmdirs = foreach ($dir in $dirs ) {
    $n = 'rm' + $dir.Replace($Root, '').Replace('\', '-')

    $Path |
        Add-Build $dir -Rule $misc.MakeDir |
        Add-Build $n -Rule $misc.Remove `
            -Variables @{$SourceVar = $dir } |
        Add-NewLine -q

    $n
}

Add-Phony -Path $Path clean @rmdirs -NewLine -Quiet

####################################################################################################
# crictl: needed for go-build move commands
####################################################################################################

if ( -not $NoCrict ) {
    try {
        $CrictlSource = Resolve-Command -Name 'crictl.exe' -Path $CrictlSource
        Write-Verbose "Using crictl `"$CrictlSource`""

        Write-Verbose 'Adding crictl rule and build declarations'
        $crictlcmds = Add-CrictlRule -Path $Path -PwshCmd $PwshCmd `
            -CrictlFlags $CrictlFlags -CrictlSource $CrictlSource

        Add-Build -Path $Path -Build rmpods -Rule $crictlcmds.RemovePods `
            -NewLine -Quiet
    } catch {
        Write-Warning "$_ Skipping crictl rules."
        $NoCrictl = $True
    }
}

####################################################################################################
# go
####################################################################################################

$gocmds = Add-GoRule -Path $Path -PwshCmd $PwshCmd -GoSource $GoSource `
    -GoFlags $GoFlags -GoBuildFlags $GoBuildFlags -GoTestFlags $GoTestFlags

$Path |
    Add-Phony vend vendmain vendtest -Default |
    Add-NewLine |
    Add-Build vendmain -Rule $gocmds.Vendor '.' |
    Add-Build vendtest -Rule $gocmds.Vendor '.\test' |
    Add-NewLine -q

####################################################################################################
# go general
####################################################################################################

Write-Verbose 'Adding general go dependencies'

$deps = foreach ( $d in @(
        @{URL = 'golang.org/x/perf/cmd/benchseries'; Version = 'latest' }
        @{URL = 'golang.org/x/perf/cmd/benchstat'; Version = 'latest' }
    )) {
    $n = Split-Path $d['URL'] -Leaf
    $p = Join-Path $GoPath 'bin' ($n + $GoExe['windows'])

    Add-Build -Path $Path -Build $p -Rule $gocmds.Install `
        -Variables @{$UrlVar = $d['URL'] ; $VersionVar = $d['Version'] } -Quiet

    $p
}


Add-Phony -Path $Path -Build go-deps @deps -NewLine -Quiet
$AllDeps += 'go-deps'

####################################################################################################
# go builds
####################################################################################################

Write-Verbose 'Adding go build declarations'

$builds = @(
    @{Name = 'shim'; Source = 'cmd\containerd-shim-runhcs-v1'; Move = $CPlatPath }
    @{Source = 'cmd\runhcs'; Dest = $CmdsBin }
    @{Source = 'cmd\ncproxy'; Dest = $CmdsBin }
    @{Source = 'cmd\wclayer'; Dest = $CmdsBin }
    @{Source = 'cmd\tar2ext4'; Dest = $CmdsBin }
    @{Source = 'cmd\shimdiag'; Dest = $CmdsBin }

    @{Source = 'internal\tools\uvmboot'; Dest = $ToolsBin }
    @{Source = 'internal\tools\zapdir'; Dest = $ToolsBin }

    @{Source = 'cmd\gcs'; Dest = $CmdsBin; GOOS = 'linux' }
    @{Source = 'cmd\gcstools'; Dest = $CmdsBin; GOOS = 'linux' }
    @{Source = 'cmd\hooks\wait-paths'; Dest = $CmdsBin; GOOS = 'linux' }
)

Add-Comment -Path $Path go executable build declarations -NewLine -Quiet

foreach ( $build in $builds ) {
    $build['Source'] = Join-Path $Root $build['Source']
    ( $build['Dest'] -or ($build['Dest'] = $Root)) > $null
    ( $build['Name'] -or ($build['Name'] = Split-Path $build['Source'] -Leaf)) > $null

    if ( $build['Move'] -and ( -not $moverules ) ) {
        $build['Move'] = $null
    }

    Add-GoBuildDeclaration -Path $Path -Rule $gocmds.Build @build `
        -MoveImplicit (($NoCrictl) ? '' : $crictlcmds.RemovePods) `
        -MoveRule $misc.Move > $null
    Add-NewLine -Path $Path -Quiet
}

Add-Phony -Path $Path `
    -Build tools uvmboot zapdir tar2ext4 shimdiag `
    -NewLine -q

####################################################################################################
# go test builds
####################################################################################################

Write-Verbose 'Adding go test build declarations'

$test_builds = @(
    @{Name = 'shimtest'; Source = 'test\containerd-shim-runhcs-v1'; Dest = $TestsBin }
    @{Name = 'critest'; Source = 'test\cri-containerd'; Dest = $TestsBin }
    @{Source = 'test\functional'; Dest = $TestsBin }
    @{Name = 'runhcstest'; Source = 'test\runhcs'; Dest = $TestsBin }

    @{Name = 'gcstest'; Source = 'test\gcs'; Dest = $TestsBin; GOOS = 'linux' }
)

Add-Comment -Path $Path go test-executable build declarations -NewLine -Quiet

$tests = foreach ( $build in $test_builds ) {
    $build['Source'] = Join-Path $Root $build['Source']
    ( $build['Dest'] -or ($build['Dest'] = $Root)) > $null
    ( $build['Name'] -or ($build['Name'] = Split-Path $build['Source'] -Leaf)) > $null

    if ( $build['Move'] -and ( -not $moverules ) ) {
        $build['Move'] = $null
    }

    $t = Add-GoBuildDeclaration -Path $Path -Rule $gocmds.TestBuild @build `
        -MoveImplicit (($NoCrictl) ? '' : $crictlcmds.RemovePods) `
        -MoveRule $misc.Move

    if ( $t ) { Add-NewLine -Path $Path -Quiet }
    $t
}

Add-Phony -Path $Path tests @tests -NewLine -Quiet

####################################################################################################
# go generate
####################################################################################################

Write-Verbose 'Adding go generate build declarations'

$gogen_deps = foreach ( $d in @(
        @{URL = 'github.com/josephspurrier/goversioninfo/cmd/goversioninfo'; Version = 'latest' }
    )) {
    $n = Split-Path $d['URL'] -Leaf
    $p = Join-Path $GoPath 'bin' ($n + $GoExe['windows'])

    Add-Build -Path $Path -Build $p -Rule $gocmds.Install `
        -Variables @{$UrlVar = $d['URL'] ; $VersionVar = $d['Version'] } -Quiet

    $p
}

Add-Phony -Path $Path -Build go-gen-deps -Value $gogen_deps -NewLine -Quiet
$AllDeps += 'go-gen-deps'

Write-Verbose 'Creating go generate declarations for go packages files'
$gens = foreach ( $pkg in Get-GoGenPackage $Root ) {
    $name = "gogen" + $pkg.Replace($Root, '').Replace('\', '-')

    Add-Build -Path $Path -Rule $gocmds.Generate -Build $name `
        -OrderOnly go-gen-deps `
        $pkg -Quiet

    $name
}

$Path |
    Add-NewLine |
    Add-Phony -Build go-gen @gens -NewLine -Quiet

####################################################################################################
# proto
####################################################################################################

if ( -not $NoProto ) {
    Write-Verbose 'Adding protobuf variables, rules, and builds'

    $ProtobuildModule = 'github.com/containerd/protobuild'
    $ProtocURL = 'https://github.com/protocolbuffers/protobuf/releases/download/' + `
        "v$ProtocVersion/protoc-$ProtocVersion-win32.zip"
    $ProtocIncludeStamp = Join-Path $DepsDir 'protoc.include.stamp'
    $ProtocIncludeDynDep = Join-Path $DepsDir 'protoc.include.dd'
    $ProtoEnv = [string[]]''
    $ProtoDeps = [string[]]$ProtocIncludeStamp

    $ProtoCmd = "& $(fv $ProtobuildVar "'")"

    $Path |
        Add-Comment |
        Add-Comment protobuild |
        Add-Comment -NewLine |

        # variables

        Add-Variable $ProtobuildVar $ProtobuildSource -NewLine |

        # install dependencies

        Add-Comment download and unzip protoc.zip |
        Add-Build $ProtocZip -Rule $misc.Download `
            -OrderOnly (Split-Path $ProtocZip) `
            -Variables @{$UrlVar = $ProtocURL } |
        Add-NewLine |

        Add-Build $ProtocIncludeDynDep -Rule $misc.TarDD $ProtocZip `
            -OrderOnly ($ProtoDir, (Split-Path $ProtocIncludeDynDep)) `
            -Variables @{
            $StampVar     = $ProtocIncludeStamp
            $StripCompVar = 1
            $DestVar      = $ProtoDir
        } |
        Add-NewLine |
        Add-Comment extract include files |
        Add-Build $ProtocIncludeStamp -Rule $misc.Tar $ProtocZip `
            -OrderOnly ($ProtoDir, (Split-Path $ProtocIncludeStamp), $ProtocIncludeDynDep) `
            -DynDep $ProtocIncludeDynDep `
            -Variables @{$CmdFlagsVar = "-xm -C $ProtoDir --strip-components 1 include" } |
        Add-NewLine -Quiet

    if ( $installprotoc ) {
        Write-Verbose 'Adding protoc.exe build declaration'
        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path $ProtocSource)'", ';')
        $ProtoDeps += $ProtocSource

        $Path |
            Add-Comment install protoc.exe -NewLine |
            Add-Build '' -Rule $misc.Tar $ProtocZip `
                -ImplicitOutput $ProtocSource `
                -OrderOnly (Split-Path $ProtocSource) `
                -Variables @{$CmdFlagsVar = "-xm -C '$(Split-Path $ProtocSource)' --strip-components 1 bin" } |
            Add-NewLine -q
    }

    if ( $installprotobuild ) {
        Write-Verbose 'adding protobuild installation build'
        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path $ProtobuildSource )'", ';')
        $ProtoDeps += (fv $ProtobuildVar)

        $Path |
            Add-Comment install protobuild -NewLine |
            Add-Build (fv $ProtobuildVar) -Rule go-install `
                -Variables @{$UrlVar = $ProtobuildModule ; $VersionVar = "v$ProtobuildVersion" } |
            Add-NewLine -q
    }

    Add-Phony -Path $Path -Build proto-deps @ProtoDeps -NewLine -Quiet
    $AllDeps += 'proto-deps'

    # protobuild proper
    Add-Rule -Path $Path -Rule protobuild `
        -Description ('building proto files', (fv $ModuleVar -q '"'), 'with flags:', `
            $ProtobuildFlags, (fv $ProtobuildFlagsVar `")) `
        @PwshCmd @ProtoEnv $ProtoCmd $ProtobuildFlags (fv $ProtobuildFlagsVar) (fv $ModuleVar -q "'") `
        -NewLine -Quiet

    Write-Verbose 'Creating protobuild declarations for *.proto files'
    $protos = foreach ( $f in Get-ProtoFile -Module $Root ) {
        $Proto = $f.replace('.proto', '.pb.go')
        $Dir = (Split-Path -Parent $f)
        $M = Get-GoModuleName -Path $Dir

        Add-Build -Path $Path -Rule protobuild -Build $Proto `
            -Implicit $f `
            -OrderOnly 'proto-deps' `
            -Variables @{$ModuleVar = $M } `
            -NewLine -Quiet

        $Proto
    }
    Add-Phony -Path $Path -Build proto @protos -NewLine -Quiet
}

####################################################################################################
# dependencies
####################################################################################################

Add-Phony -Path $Path deps @AllDeps -Quiet
