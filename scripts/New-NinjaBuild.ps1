[CmdletBinding(SupportsShouldProcess)]
param (
    [string]
    # The path to the build file. Do not include `~` in the path
    $Path = '.\build.ninja',

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

Import-Module ( Join-Path $PSScriptRoot NinjaBuild ) -Force

if ( $Info ) {
    Get-Help Update-NinjaFile

    Write-Output '' ''

    (Get-Command Update-NinjaFile).ParameterSets |
        Select-Object -Property @{n = 'ParameterSetName'; e = { $_.name } },
        @{n = 'Parameters'; e = { $_.ToString() } }

    return
}

####################################################################################################
# setup
####################################################################################################

Write-Verbose 'Resolving paths and commands'

if ( -not (Test-Path $Path -IsValid) ) {
    throw "`"$Path`" is not a valid path"
}
if ( -not ( [System.IO.Path]::IsPathRooted($Path) ) ) {
    $Path = Join-Path (Get-Location) $Path
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

$Bin = 'bin'
$CmdsBin = "$Bin\cmd"
$ToolsBin = "$Bin\tools"
$TestsBin = "$Bin\test"
$OutDir = 'out'
$DepsDir = 'deps'
$dirs = [string[]]($CmdsBin, $ToolsBin, $TestsBin, $OutDir, $DepsDir)

$PwshSource = Resolve-Command -Name 'pwsh.exe' -Path $PwshSource
Write-Verbose "Using powershell `"$PwshSource`""

$GoSource = Resolve-Command -Name 'go.exe' -Path $GoSource
Write-Verbose "Using go `"$GoSource`""

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

$ProtoDir = 'protobuf'
$ProtoBin = 'bin\protobuf'

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

New-NinjaBuildFile -Path $Path -GoModule $GoModule -CreatedBy $PSCommandPath -Quiet

$PwshCmd = Add-PwshRule -Path $Path -Source $PwshSource `
    -Flags (('-NoProfile', '-NoLogo', '-NonInteractive') + $PwshFlags)
$misc = Add-MiscRule -Path $Path -PwshCmd $PwshCmd

####################################################################################################
# installation directories
####################################################################################################

$dirs = $dirs | Get-Unique
foreach ($dir in $dirs ) {
    $Path |
        Update-NinjaFile -Build $dir -Rule $misc.MakeDir |
        Update-NinjaFile -Build "rm-$($dir.Replace('\','-'))" -Rule $misc.Remove `
            -Variables @{$DestVar = $dir } |
        Update-NinjaFile -NewLine -q
}

$Path |
    Update-NinjaFile -Build clean -Rule phony @($dirs | ForEach-Object { "rm-$($_.Replace('\','-'))" }) |
    Update-NinjaFile -NewLine -q

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

        Update-NinjaFile -Path $Path -Build rmpods -Rule $crictlcmds.RemovePods `
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
    Update-NinjaFile -Build vend -Rule phony vendmain vendtest -Default |
    Update-NinjaFile -NewLine |
    Update-NinjaFile -Build vendmain -Rule $gocmds.Vendor '.' |
    Update-NinjaFile -Build vendtest -Rule $gocmds.Vendor '.\test' |
    Update-NinjaFile -NewLine -q

####################################################################################################
# go builds
####################################################################################################

Write-Verbose 'Adding go build declarations'

$builds = @(
    @{Name = 'shim'; Source = 'cmd\containerd-shim-runhcs-v1'; Move = $CPlatPath }
    @{Name = 'runhcs'; Source = 'cmd\runhcs'; Dest = $CmdsBin }
    @{Name = 'ncproxy'; Source = 'cmd\ncproxy'; Dest = $CmdsBin }
    @{Name = 'wclayer'; Source = 'cmd\wclayer'; Dest = $CmdsBin }
    @{Name = 'tar2ext4'; Source = 'cmd\tar2ext4'; Dest = $CmdsBin }
    @{Name = 'shimdiag'; Source = 'cmd\shimdiag'; Dest = $CmdsBin }

    @{Name = 'uvmboot'; Source = 'internal\tools\uvmboot'; Dest = $ToolsBin }
    @{Name = 'zapdir'; Source = 'internal\tools\zapdir'; Dest = $ToolsBin }

    @{Name = 'gcs'; Source = 'cmd\gcs'; Dest = $CmdsBin; GOOS = 'linux' }
)

Update-NinjaFile -Path $Path -Comment go executable build declarations -NewLine -Quiet

foreach ( $build in $builds ) {
    if ( $build['Move'] -and ( -not $moverules ) ) {
        $build['Move'] = $null
    }

    Add-GoBuildDeclaration -Path $Path -Rule $gocmds.Build @build `
        -MoveImplicit (($NoCrictl) ? '' : $crictlcmds.RemovePods) `
        -MoveRule $misc.Move > $null
    Update-NinjaFile -Path $Path -NewLine -Quiet
}

Update-NinjaFile -Path $Path `
    -Build tools -Rule phony uvmboot zapdir tar2ext4 shimdiag `
    -NewLine -q

####################################################################################################
# go test builds
####################################################################################################

Write-Verbose 'Adding go test build declarations'

$test_builds = @(
    @{Name = 'shimtest'; Source = 'test\containerd-shim-runhcs-v1' } #; Dest = $TestsBin }
    @{Name = 'critest'; Source = 'test\cri-containerd'; Dest = $TestsBin }
    @{Name = 'functional'; Source = 'test\functional'; Dest = $TestsBin }
    @{Name = 'runhcstest'; Source = 'test\runhcs'; Dest = $TestsBin }
    @{Name = 'gcstest'; Source = 'test\gcs'; Dest = $TestsBin; GOOS = 'linux' }
)

Update-NinjaFile -Path $Path -Comment go test-executable build declarations -NewLine -Quiet

$tests = foreach ( $build in $test_builds ) {
    if ( $build['Move'] -and ( -not $moverules ) ) {
        $build['Move'] = $null
    }

    Add-GoBuildDeclaration -Path $Path -Rule $gocmds.TestBuild @build `
        -MoveImplicit (($NoCrictl) ? '' : 'rmpods')
    Update-NinjaFile -Path $Path -NewLine -Quiet
}

Update-NinjaFile -Path $Path -Build tests -Rule phony @tests -NewLine -Quiet

####################################################################################################
# go generate
####################################################################################################

Write-Verbose 'Adding go generate build declarations'

$gogen_deps = @(
    @{URL = 'github.com/josephspurrier/goversioninfo/cmd/goversioninfo'; Version = 'latest' }
)

foreach ( $d in $gogen_deps ) {
    $n = Split-Path $d['URL'] -Leaf
    $d['Name'] = $n
    $d['Path'] = Join-Path $GoPath 'bin' ($n + $GoExe['windows'])

    Update-NinjaFile -Path $Path -Build $d['Path'] -Rule $gocmds.Install `
        -Variables @{$UrlVar = $d['URL'] ; $VersionVar = $d['Version'] } -Quiet
}

Update-NinjaFile -Path $Path -NewLine -Quiet

Write-Verbose 'Creating go generat declarations for go packages files'
$gens = foreach ( $pkg in Get-GoGenPackage '.' ) {
    $name = "gogen-$($pkg.Replace('\','-'))"

    Update-NinjaFile -Path $Path -Rule $gocmds.Generate -Build $name `
        -OrderOnly ($gogen_deps | ForEach-Object { $_['Path'] }) `
        $pkg -Quiet

    $name
}

$Path |
    Update-NinjaFile -NewLine |
    Update-NinjaFile -Build go-gen -Rule phony @gens -NewLine -Quiet

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
        Update-NinjaFile -Comment |
        Update-NinjaFile -Comment protobuild |
        Update-NinjaFile -Comment |
        Update-NinjaFile -NewLine |

        # variables

        Update-NinjaFile -Variable $ProtobuildVar $ProtobuildSource |
        Update-NinjaFile -Variable $ProtobuildFlagsVar |
        Update-NinjaFile -NewLine |

        # install dependencies

        Update-NinjaFile -Comment download and unzip protoc.zip |
        Update-NinjaFile -Build $ProtocZip -Rule $misc.Download `
            -OrderOnly (Split-Path $ProtocZip) `
            -Variables @{$UrlVar = $ProtocURL } |
        Update-NinjaFile -NewLine |

        Update-NinjaFile -Build $ProtocIncludeDynDep -Rule $misc.TarDD $ProtocZip `
            -OrderOnly ($ProtoDir, (Split-Path $ProtocIncludeDynDep)) `
            -Variables @{
            $StampVar     = $ProtocIncludeStamp
            $StripCompVar = 1
            $DestVar      = $ProtoDir
        } |
        Update-NinjaFile -NewLine |
        Update-NinjaFile -Comment extract include files |
        Update-NinjaFile -Build $ProtocIncludeStamp -Rule $misc.Tar $ProtocZip `
            -OrderOnly ($ProtoDir, (Split-Path $ProtocIncludeStamp), $ProtocIncludeDynDep) `
            -DynDep $ProtocIncludeDynDep `
            -Variables @{$CmdFlagsVar = "-xm -C $ProtoDir --strip-components 1 include" } |
        Update-NinjaFile -NewLine -Quiet

    if ( $installprotoc ) {
        Write-Verbose 'Adding protoc.exe build declaration'
        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path (Join-Path (Resolve-Path .) $ProtocSource))'", ';')
        $ProtoDeps += $ProtocSource

        $Path |
            Update-NinjaFile -Comment install protoc.exe -NewLine |
            Update-NinjaFile -Build '' -Rule $misc.Tar $ProtocZip `
                -ImplicitOutputs $ProtocSource `
                -OrderOnly (Split-Path $ProtocSource) `
                -Variables @{$CmdFlagsVar = "-xm -C '$(Split-Path $ProtocSource)' --strip-components 1 bin" } |
            Update-NinjaFile -NewLine -q
    }

    if ( $installprotobuild ) {
        Write-Verbose 'adding protobuild installation build'
        $ProtoEnv += ('$$env:Path +=', "';$(Split-Path $ProtobuildSource )'", ';')
        $ProtoDeps += (fv $ProtobuildVar)

        $Path |
            Update-NinjaFile -Comment install protobuild -NewLine |
            Update-NinjaFile -Build (fv $ProtobuildVar) -Rule go-install `
                -Variables @{$UrlVar = $ProtobuildModule ; $VersionVar = "v$ProtobuildVersion" } |
            Update-NinjaFile -NewLine -q
    }

    Update-NinjaFile -Path $Path -Build proto-deps -Rule phony @ProtoDeps -NewLine -Quiet

    # protobuild proper
    Update-NinjaFile -Path $Path -Rule protobuild `
        -Description ('building proto files', (fv $ModuleVar -q '"'), 'with flags:', `
            $ProtobuildFlags, (fv $ProtobuildFlagsVar `")) `
        @PwshCmd @ProtoEnv $ProtoCmd $ProtobuildFlags (fv $ProtobuildFlagsVar) (fv $ModuleVar -q "'") `
        -NewLine -Quiet

    Write-Verbose 'Creating protobuild declarations for *.proto files'
    $protos = foreach ( $f in Get-ProtoFile -Module '.') {
        $Proto = $f.replace('.proto', '.pb.go')
        $Dir = (Split-Path -Parent $f)

        $M = Get-GoModuleName -Path ".\$Dir"
        Update-NinjaFile -Path $Path -Rule protobuild -Build $Proto `
            -Implicit $f `
            -OrderOnly 'proto-deps' `
            -Variables @{$ModuleVar = $M } `
            -NewLine -Quiet

        $Proto
    }
    Update-NinjaFile -Path $Path -Build proto -Rule phony @protos -NewLine -Quiet
}
