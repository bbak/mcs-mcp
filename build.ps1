param (
	[Parameter(Mandatory = $false)]
	[string]$Command = "build",
	[Parameter(Mandatory = $false)]
	[string]$Version = (Get-Content "$PSScriptRoot/VERSION").Trim()
)

$DistDir = "dist"
$BinaryName = "$DistDir/mcs-mcp.exe"
$Pkg = "mcs-mcp/cmd/mcs-mcp/commands"
$Commit = git rev-parse --short HEAD 2>$null
if ($null -eq $Commit) { $Commit = "none" }
if (git status --porcelain) { $Commit += "-dirty" }
$BuildDate = Get-Date -Format "yyyy-MM-ddTHH:mm:ss"
$LdFlags = "-s -w -X $Pkg.Version=$Version -X $Pkg.Commit=$Commit -X $Pkg.BuildDate=$BuildDate"

switch ($Command) {
	"build" {
		Write-Host "Building $BinaryName..." -ForegroundColor Cyan
		if (-not (Test-Path $DistDir)) { New-Item -ItemType Directory $DistDir | Out-Null }

		Write-Host "Generating version info resource..." -ForegroundColor Gray
		Push-Location cmd/mcs-mcp
		# Try to run goversioninfo, ignore error if not found (but log it)
		try {
			# Parse version string (e.g., 0.24.2)
			$v = $Version.Split(".")
			$major = if ($v.Length -gt 0) { $v[0] } else { 0 }
			$minor = if ($v.Length -gt 1) { $v[1] } else { 0 }
			$patch = if ($v.Length -gt 2) { $v[2] } else { 0 }

			& goversioninfo -platform-specific -ver-major $major -ver-minor $minor -ver-patch $patch -file-version $Version -product-version $Version
		}
		catch {
			Write-Warning "goversioninfo failed or not found. File metadata will not be populated."
		}
		Pop-Location

		go build -ldflags "$LdFlags" -o $BinaryName ./cmd/mcs-mcp

		Write-Host "Building $DistDir/mockgen.exe..." -ForegroundColor Cyan
		go build -ldflags "-s -w" -o "$DistDir/mockgen.exe" ./cmd/mockgen

		Write-Host "Copying example configs to $DistDir..." -ForegroundColor Cyan
		Copy-Item "conf/.env-example" -Destination "$DistDir/.env-example" -Force
	}
	"test" {
		Write-Host "Running tests..." -ForegroundColor Cyan
		go test -v ./...
	}
	"verify" {
		Write-Host "Verifying project..." -ForegroundColor Cyan
		Write-Host "Checking fmt..." -NoNewline
		go fmt ./...
		Write-Host "Checking lint..." -NoNewline
		golangci-lint run
		Write-Host "Running tests..." -NoNewline
		go test -v ./...
	}
	"clean" {
		Write-Host "Cleaning..." -ForegroundColor Cyan
		if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }
		if (Test-Path "cmd/mcs-mcp/resource.syso") { Remove-Item "cmd/mcs-mcp/resource.syso" }
		go clean
	}
	"help" {
		Write-Host "Usage: ./build.ps1 [command] [version]" -ForegroundColor Yellow
		Write-Host "Commands:"
		Write-Host "  build    Build the binary (default)"
		Write-Host "  test     Run unit tests"
		Write-Host "  verify   Run fmt, lint, and test"
		Write-Host "  clean    Remove build artifacts"
		Write-Host "  help     Show this help"
	}
	Default {
		Write-Error "Unknown command: $Command. Use 'help' for usage."
	}
}
