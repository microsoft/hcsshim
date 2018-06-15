<#
.NOTES
    Summary: Simple wrapper to build a local initrd.img, rootfs.tar.gz and rootfs.vhd from sources and optionally install it.

    License: See https://github.com/Microsoft/opengcs/blob/master/LICENSE

.Parameter Install
    Installs the built initrd.img and rootfs.vhd

#>


param(
    [Parameter(Mandatory=$false)][switch]$Install
)

$ErrorActionPreference = 'Stop'

function New-TemporaryDirectory {
    $parent = [System.IO.Path]::GetTempPath()
    [string] $name = [System.Guid]::NewGuid()
    New-Item -ItemType Directory -Path (Join-Path $parent $name)
}

Try {
    Write-Host -ForegroundColor Yellow "INFO: Starting at $(date)`n"

    &docker build --platform=linux -t opengcs .
    if ( $LastExitCode -ne 0 ) {
        Throw "failed to build opengcs image"
    }

    $d=New-TemporaryDirectory
    Write-Host -ForegroundColor Yellow "INFO: Compiling GCS and rootfs2vhd binaries"
    docker run --rm -v $d`:/build/out opengcs
    if ( $LastExitCode -ne 0 ) {
        Throw "failed to build"
    }

	if ([environment]::OSVersion.Version.Build -gt 17134) {
		Write-Host -ForegroundColor Yellow "INFO: Generating rootfs.vhd"
		pushd $d
		.\rootfs2vhd
		popd
		if ( $LastExitCode -ne 0 ) {
			Write-Warning "failed to convert to rootfs to VHD. Ignoring while tool still in progress"
		}
	} else {
		Write-Warning "Skipping conversion of root file system to VHD - requires RS5+"
	}
	

    if ($Install) {
        if (Test-Path "C:\Program Files\Linux Containers\initrd.img" -PathType Leaf) {
            copy "C:\Program Files\Linux Containers\initrd.img" "C:\Program Files\Linux Containers\initrd.old"
            Write-Host -ForegroundColor Yellow "INFO: Backed up previous initrd.img to C:\Program Files\Linux Containers\initrd.old"
        }
        copy "$d`\initrd.img" "C:\Program Files\Linux Containers\initrd.img"

        if (Test-Path "$d`\rootfs.vhd" -PathType Leaf) {
			if (Test-Path "C:\Program Files\Linux Containers\rootfs.vhd" -PathType Leaf) {
				copy "C:\Program Files\Linux Containers\rootfs.vhd" "C:\Program Files\Linux Containers\rootfs.old"
				Write-Host -ForegroundColor Yellow "INFO: Backed up previous rootfs.vhd to C:\Program Files\Linux Containers\rootfs.old"
			}
			copy "$d`\rootfs.vhd" "C:\Program Files\Linux Containers\rootfs.vhd"
		}

        Write-Host -ForegroundColor Yellow "INFO: Restart the docker daemon to pick up the new filee"
	}

	Write-Host -ForegroundColor Yellow "INFO: Targets in $d"
}
Catch [Exception] {
    Throw $_
}
Finally {
    Write-Host -ForegroundColor Yellow "INFO: Exiting at $(date)"
}
