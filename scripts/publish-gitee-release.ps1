#Requires -Version 7.0

param(
  [Parameter(Mandatory = $true)]
  [string]$Version,
  [string]$Repository = 'Zcy-sa/auth-pro',
  [string]$Remote = 'origin',
  [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
  throw "Version must match X.Y.Z: $Version"
}
if ($Repository -notmatch '^([^/]+)/([^/]+)$') {
  throw "Repository must match owner/repo: $Repository"
}

$AccessToken = $env:GITEE_ACCESS_TOKEN
if ([string]::IsNullOrWhiteSpace($AccessToken)) {
  throw 'GITEE_ACCESS_TOKEN is required'
}

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$Owner = $Matches[1]
$Repo = $Matches[2]
$Tag = "v$Version"
$EncodedOwner = [Uri]::EscapeDataString($Owner)
$EncodedRepo = [Uri]::EscapeDataString($Repo)
$EncodedTag = [Uri]::EscapeDataString($Tag)
$ApiBase = "https://gitee.com/api/v5/repos/$EncodedOwner/$EncodedRepo"
$PackagesDir = Join-Path $Root 'release/packages'
$PackagePath = Join-Path $PackagesDir "auth_pro-full-v$Version.tar.gz"
$LatestPath = Join-Path $PackagesDir 'latest.json'
$ReleasesPath = Join-Path $PackagesDir 'releases.json'

function Invoke-Git {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)

  $output = & git @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "git $($Arguments -join ' ') failed: $($output -join [Environment]::NewLine)"
  }
  return ($output -join [Environment]::NewLine).Trim()
}

function Test-NotFound {
  param([Parameter(Mandatory = $true)]$ErrorRecord)

  return $null -ne $ErrorRecord.Exception.Response -and
    [int]$ErrorRecord.Exception.Response.StatusCode -eq 404
}

function Get-GiteeRelease {
  param([Parameter(Mandatory = $true)][string]$Uri)

  try {
    return Invoke-RestMethod -Uri $Uri -Method Get
  } catch {
    if (Test-NotFound $_) {
      return $null
    }
    throw
  }
}

Push-Location $Root
try {
  $status = Invoke-Git @('status', '--porcelain')
  if ($status) {
    throw 'Working tree must be clean before publishing a release'
  }

  $remoteUrl = Invoke-Git @('remote', 'get-url', $Remote)
  if ($remoteUrl -notmatch "(?i)gitee\.com[/:]$([Regex]::Escape($Owner))/$([Regex]::Escape($Repo))(?:\.git)?$") {
    throw "Remote $Remote does not point to https://gitee.com/${Repository}: $remoteUrl"
  }

  $headCommit = Invoke-Git @('rev-parse', 'HEAD')
  $tagCommit = Invoke-Git @('rev-list', '-n', '1', $Tag)
  if ($headCommit -ne $tagCommit) {
    throw "Tag $Tag must point to HEAD before publishing"
  }
  Invoke-Git @('ls-remote', '--exit-code', '--tags', $Remote, "refs/tags/$Tag") | Out-Null

  $existingRelease = Get-GiteeRelease "$ApiBase/releases/tags/$EncodedTag"
  if ($null -ne $existingRelease) {
    throw "Gitee Release $Tag already exists"
  }

  New-Item -ItemType Directory -Force -Path $PackagesDir | Out-Null
  $latestRelease = Get-GiteeRelease "$ApiBase/releases/latest"
  if ($null -ne $latestRelease) {
    $attachments = Invoke-RestMethod -Uri "$ApiBase/releases/$($latestRelease.id)/attach_files" -Method Get
    $historyAttachment = $attachments | Where-Object { $_.name -eq 'releases.json' } | Select-Object -First 1
    if ($null -ne $historyAttachment) {
      Invoke-WebRequest -Uri $historyAttachment.browser_download_url -OutFile $ReleasesPath
    } else {
      Remove-Item -LiteralPath $ReleasesPath -Force -ErrorAction SilentlyContinue
    }
  } else {
    Remove-Item -LiteralPath $ReleasesPath -Force -ErrorAction SilentlyContinue
  }

  $environmentNames = @(
    'AUTO_PRO_RELEASE_REPOSITORY',
    'AUTO_PRO_UPDATE_PACKAGE_BASE_URL',
    'AUTO_PRO_UPDATE_RELEASES_URL',
    'AUTO_PRO_REQUIRE_GIT_RELEASE_NOTES',
    'AUTO_PRO_RELEASE_CURRENT_REF'
  )
  $previousEnvironment = @{}
  foreach ($name in $environmentNames) {
    $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
  }
  try {
    $env:AUTO_PRO_RELEASE_REPOSITORY = $Repository
    $env:AUTO_PRO_UPDATE_PACKAGE_BASE_URL = "https://gitee.com/$Repository/releases/download/$Tag"
    $env:AUTO_PRO_UPDATE_RELEASES_URL = "https://gitee.com/$Repository/releases/download/$Tag/releases.json"
    $env:AUTO_PRO_REQUIRE_GIT_RELEASE_NOTES = '1'
    $env:AUTO_PRO_RELEASE_CURRENT_REF = $Tag
    & (Join-Path $PSScriptRoot 'build-release.ps1') -Version $Version
    if ($LASTEXITCODE -ne 0) {
      throw 'Release build failed'
    }
  } finally {
    foreach ($name in $environmentNames) {
      [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process')
    }
  }

  foreach ($path in @($PackagePath, $LatestPath, $ReleasesPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -eq 0) {
      throw "Release asset was not generated: $path"
    }
  }

  if (-not $SkipTests) {
    Write-Host '[verify] Running backend tests...'
    & go -C (Join-Path $Root 'backend') test ./...
    if ($LASTEXITCODE -ne 0) {
      throw 'Backend tests failed'
    }
  }

  $manifest = Get-Content -LiteralPath $LatestPath -Raw | ConvertFrom-Json
  $releaseBody = ($manifest.notes | ForEach-Object { "- $_" }) -join [Environment]::NewLine
  if ([string]::IsNullOrWhiteSpace($releaseBody)) {
    $releaseBody = "Version $Version"
  }

  Write-Host "Creating Gitee Release $Tag..."
  $release = Invoke-RestMethod -Uri "$ApiBase/releases" -Method Post -Body @{
    access_token = $AccessToken
    tag_name = $Tag
    name = $Tag
    body = $releaseBody
    prerelease = 'false'
    target_commitish = $headCommit
  }

  try {
    foreach ($path in @($PackagePath, $LatestPath, $ReleasesPath)) {
      Write-Host "Uploading $(Split-Path -Leaf $path)..."
      Invoke-RestMethod -Uri "$ApiBase/releases/$($release.id)/attach_files" -Method Post -Form @{
        access_token = $AccessToken
        file = Get-Item -LiteralPath $path
      } | Out-Null
    }
  } catch {
    $publishError = $_
    try {
      Invoke-RestMethod -Uri "$ApiBase/releases/$($release.id)" -Method Delete -Body @{
        access_token = $AccessToken
      } | Out-Null
    } catch {
      Write-Warning "Failed to remove incomplete Gitee Release $Tag"
    }
    throw $publishError
  }

  Write-Host "Published: https://gitee.com/$Repository/releases/tag/$Tag"
} finally {
  Pop-Location
}
