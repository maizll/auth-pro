param(
  [string]$Version = ""
)

$ErrorActionPreference = 'Stop'

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$VersionFile = Join-Path $Root 'VERSION'
if ([string]::IsNullOrWhiteSpace($Version) -and (Test-Path $VersionFile)) {
  $Version = (Get-Content -LiteralPath $VersionFile -Raw).Trim()
}
if ([string]::IsNullOrWhiteSpace($Version)) {
  $Version = '1.5.6'
}
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
  throw "Version must match X.Y.Z: $Version"
}

$FrontendDir = Join-Path $Root 'frontend'
$BackendDir = Join-Path $Root 'backend'
$DistName = "auth_pro-full-v$Version"
$ReleaseRoot = Join-Path $Root 'release'
$PackageDir = Join-Path $ReleaseRoot $DistName
$PackagesDir = Join-Path $ReleaseRoot 'packages'
$PackagePath = Join-Path $PackagesDir "$DistName.tar.gz"
$LatestPath = Join-Path $PackagesDir 'latest.json'
$ReleasesPath = Join-Path $PackagesDir 'releases.json'
$ReleaseRepository = if ($env:AUTO_PRO_RELEASE_REPOSITORY) { $env:AUTO_PRO_RELEASE_REPOSITORY } else { 'maizll/auth-pro-client' }
$UpdatePackageBaseUrl = if ($env:AUTO_PRO_UPDATE_PACKAGE_BASE_URL) { $env:AUTO_PRO_UPDATE_PACKAGE_BASE_URL } else { "https://github.com/$ReleaseRepository/releases/download/v$Version" }
$UpdateReleasesUrl = if ($env:AUTO_PRO_UPDATE_RELEASES_URL) { $env:AUTO_PRO_UPDATE_RELEASES_URL } else { "https://github.com/$ReleaseRepository/releases/download/v$Version/releases.json" }

if ($PackageDir -notlike "$Root*") {
  throw "Invalid package directory: $PackageDir"
}
if (Test-Path $PackageDir) {
  Remove-Item -LiteralPath $PackageDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $PackageDir, $PackagesDir | Out-Null

Write-Host "[1/5] Building frontend..."
Push-Location $FrontendDir
$PreviousViteVersion = $env:VITE_VERSION
try {
  $env:VITE_VERSION = $Version
  pnpm run build
} finally {
  if ($null -eq $PreviousViteVersion) {
    Remove-Item Env:VITE_VERSION -ErrorAction SilentlyContinue
  } else {
    $env:VITE_VERSION = $PreviousViteVersion
  }
  Pop-Location
}

$FrontendDist = Join-Path $FrontendDir 'dist'
if (-not (Test-Path (Join-Path $FrontendDist 'index.html'))) {
  throw "Frontend dist/index.html not found"
}
if (-not (Test-Path (Join-Path $FrontendDist 'version.json'))) {
  throw "Frontend dist/version.json not found"
}

Write-Host "[2/5] Preparing package directories..."
$PackageBackendDir = Join-Path $PackageDir 'backend'
New-Item -ItemType Directory -Force -Path $PackageBackendDir | Out-Null
Copy-Item -Path (Join-Path $FrontendDist '*') -Destination $PackageDir -Recurse -Force
foreach ($PackagedScript in @('baota-panel.py')) {
  $PackagedSource = Join-Path $Root "scripts/$PackagedScript"
  if (-not (Test-Path -LiteralPath $PackagedSource)) {
    throw "Missing packaged script: $PackagedSource"
  }
  Copy-Item -LiteralPath $PackagedSource -Destination (Join-Path $PackageDir $PackagedScript) -Force
}
$GuardianSource = Join-Path $Root 'backend/handler/guardian_start.sh'
if (-not (Test-Path -LiteralPath $GuardianSource)) {
  throw "Missing guardian start script: $GuardianSource"
}
Copy-Item -LiteralPath $GuardianSource -Destination (Join-Path $PackageDir 'guardian-start.sh') -Force

$BackendStaticDir = Join-Path $BackendDir 'static'
if ($BackendStaticDir -notlike "$Root*") {
  throw "Invalid backend static directory: $BackendStaticDir"
}
New-Item -ItemType Directory -Force -Path $BackendStaticDir | Out-Null
Get-ChildItem -LiteralPath $BackendStaticDir -Force | Remove-Item -Recurse -Force
Copy-Item -Path (Join-Path $FrontendDist '*') -Destination $BackendStaticDir -Recurse -Force

# 安装脚本的唯一源是 backend/handler/install.sh，由 go:embed 直接下发，这里不再复制。

Write-Host "[3/5] Building Linux backend..."
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$BackendBinary = Join-Path $PackageBackendDir 'auth_pro'
Push-Location $BackendDir
try {
  $env:GOOS = 'linux'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  $LdFlags = "-s -w -X auto_pro/config.AppVersion=$Version -X auto_pro/config.BuildTime=$BuildTime"
  if ($env:AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY) {
    if ($env:AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY -eq 'PLACEHOLDER_NOT_CONFIGURED') {
      throw 'AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY 仍是占位符，请改成源站 store-keygen 打印的公钥'
    }
    try {
      $PubBytes = [Convert]::FromBase64String($env:AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY)
    } catch {
      throw 'AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY 必须是 32 字节 Ed25519 公钥的标准 base64'
    }
    if ($PubBytes.Length -ne 32) {
      throw 'AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY 必须是 32 字节 Ed25519 公钥的标准 base64'
    }
    $LdFlags += " -X auto_pro/handler.embeddedStoreSnapshotPublicKey=$($env:AUTH_PRO_STORE_SNAPSHOT_PUBLIC_KEY)"
    Write-Host 'store snapshot public key: embed via ldflags'
  }
  # 本站在源站上所属的商业版应用。留空时归入源站「接收老客户端」的那个应用。
  if ($env:AUTH_PRO_STORE_PRODUCT_APP_KEY) {
    if ($env:AUTH_PRO_STORE_PRODUCT_APP_KEY -notmatch '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$') {
      throw 'AUTH_PRO_STORE_PRODUCT_APP_KEY 只能包含字母、数字、下划线和短横线'
    }
    $LdFlags += " -X auto_pro/handler.embeddedStoreProductAppKey=$($env:AUTH_PRO_STORE_PRODUCT_APP_KEY)"
    Write-Host "store product app key: $($env:AUTH_PRO_STORE_PRODUCT_APP_KEY)"
  }
  go build -trimpath -ldflags $LdFlags -o $BackendBinary .
} finally {
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}

Write-Host "[4/5] Writing signed manifest..."
# 和 build-release.sh 一样交给签名工具写 manifest.json：有 AUTH_PRO_UPDATE_SIGNING_KEY 就签名。
go -C $BackendDir run ./cmd/release-sign manifest $PackageDir $Version
if ($LASTEXITCODE -ne 0) {
  throw "Failed to write signed manifest"
}

Write-Host "[5/5] Creating tar.gz package and latest.json..."
if (Test-Path $PackagePath) {
  Remove-Item -LiteralPath $PackagePath -Force
}
tar -czf $PackagePath -C $PackageDir .

$Hash = (Get-FileHash -LiteralPath $PackagePath -Algorithm SHA256).Hash.ToLower()
$Size = (Get-Item -LiteralPath $PackagePath).Length
$PackageUrl = "$($UpdatePackageBaseUrl.TrimEnd('/'))/$DistName.tar.gz"
node (Join-Path $Root 'scripts/write-release-manifests.mjs') `
  $LatestPath `
  $ReleasesPath `
  $Version `
  $BuildTime `
  "$DistName.tar.gz" `
  $PackageUrl `
  $Hash `
  $Size `
  $UpdateReleasesUrl
if ($LASTEXITCODE -ne 0) {
  throw "Failed to write release manifests"
}

Write-Host ""
Write-Host "Release package: $PackagePath"
Write-Host "Latest manifest: $LatestPath"
Write-Host "Release history: $ReleasesPath"
Write-Host "SHA256: $Hash"
Write-Host "Size: $Size bytes"
