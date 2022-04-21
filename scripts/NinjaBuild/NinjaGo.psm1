

# global variables for commands
foreach ($d in @(
        # miscellaneous
        @{ N = 'CmdFlagsVar' ; V = 'CMD_FLAGS' }
        @{ N = 'DestVar' ; V = 'DESTINATION' }
        @{ N = 'ModuleVar' ; V = 'MODULE' }
        @{ N = 'SourceVar' ; V = 'SOURCE' }
        @{ N = 'UrlVar' ; V = 'URL' }
        @{ N = 'VersionVar' ; V = 'VERSION' }

        # powershell
        @{ N = 'PwshVar' ; V = 'PWSH' }
        @{ N = 'PwshFlagsVar' ; V = 'PWSH_FLAGS' }

        # go
        @{ N = 'GoVar' ; V = 'GO' }
        @{ N = 'GoFlagsVar' ; V = 'GO_FLAGS' }
        @{ N = 'GOOSVar' ; V = 'GOOS' }
        @{ N = 'GoExe' ; V = @{'windows' = '.exe' } }

        # crictl
        @{ N = 'CrictlVar' ; V = 'CRICTL' }
        @{ N = 'CrictlFlagsVar' ; V = 'CRICTL_FLAGS' }

        # protobuild
        @{ N = 'ProtobuildVar' ; V = 'PROTOBUILD' }
        @{ N = 'ProtobuildFlagsVar' ; V = 'PROTOBUILD_FLAGS' }
    ) ) {
    Set-Variable -Name $d['N'] -Value $d['V'] `
        -Scope Global -Option Readonly -Force `
        -ErrorAction SilentlyContinue
}

function Add-GoBuildRule {
    [CmdletBinding(PositionalBinding = $False, SupportsShouldProcess)]
    [OutputType([string])]
    param (
        [Parameter(Mandatory)]
        [string]
        $Name,

        [Parameter(Mandatory)]
        [string]
        $Rule,

        [string[]]
        $Implicit = '',

        [string[]]
        $OrderOnly = '',

        [hashtable]
        $Variables = @{},

        [string[]]
        $ImplicitOutputs = '',

        [string]
        $DynDep = '',

        [Parameter(Mandatory)]
        [string]
        $Source,

        [string]
        $Dest = '',

        [string]
        $Move = '',

        [string]
        $MoveRule = 'mv',

        [string]
        $MoveImplicit = '',

        [ValidateSet('windows', 'linux')]
        [string]
        $GOOS = 'windows',

        [Parameter(ValueFromPipeline, ValueFromPipelineByPropertyName, Mandatory)]
        [Alias('p')]
        [string]
        # The ninja build file to update.
        $Path,

        [Alias('q')]
        [switch]
        # Supress returning the path.
        $Quiet,

        $DebugPreference = $PSCmdlet.GetVariableValue('DebugPreference'),
        $VerbosePreference = $PSCmdlet.GetVariableValue('VerbosePreference'),
        $WhatIfPreference = $PSCmdlet.GetVariableValue('WhatIfPreference')
    )

    $OrderOnly = [string[]]$OrderOnly + $Dest
    $Variables[$GOOSVar] = $GOOS
    $Variables[$DestVar] = (Join-Path ( Resolve-Path '.' ) $Dest (Split-Path $Source -Leaf)) + $GoExe[$GOOS]
    $Variables[$SourceVar] = Resolve-Path $Source

    Update-NinjaFile -Path $Path -Rule $Rule -Build $Name `
        -Implicit $Implicit -ImplicitOutputs $ImplicitOutputs `
        -DynDep $DynDep `
        -OrderOnly $OrderOnly -Variables $Variables -q

    if ( $Move ) {
        $mvvars = @{
            $SourceVar   = $Variables[$DestVar]
            $DestVar     = (Join-Path (Resolve-Path $Move) (Split-Path $Variables[$DestVar] -Leaf))
            $CmdFlagsVar = '-Force'
        }

        Update-NinjaFile -Path $Path -Build "mv-$Name" -Rule $MoveRule `
            -Implicit ($Name, $MoveImplicit) `
            -Variables $mvvars -Quiet
    }

    if ( -not $Quiet ) { $Path }
}
