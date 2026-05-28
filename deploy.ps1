[CmdletBinding()]
param(
  [string]$VmUser = "dune",
  [string]$VmHost = "192.168.0.72",
  [string]$SshKeyPath = "",
  [string]$KubeconfigPath = "$HOME/.kube/dune-external.yaml",
  [string]$Image = "",
  [string]$Namespace = "dune-admin",
  [string]$Manifest = "deploy/k8s/dune-admin.rendered.yaml",
  [switch]$SkipKubeconfig,
  [switch]$SkipBuild,
  [switch]$SkipImageImport,
  [switch]$PortForward,
  [switch]$NoPortForward
)

$ErrorActionPreference = "Stop"

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Missing required command: $Name"
  }
}

function Invoke-Step([string]$Label, [scriptblock]$Action) {
  Write-Host "==> $Label"
  & $Action
}

function Get-SshOptionArgs([string]$KeyPath) {
  $args = @(
    "-o", "PreferredAuthentications=publickey,password",
    "-o", "PubkeyAuthentication=yes",
    "-o", "PasswordAuthentication=yes"
  )
  if ($KeyPath) {
    $args += @("-i", $KeyPath, "-o", "IdentitiesOnly=yes")
  }
  return $args
}

$repoRoot = Split-Path -Parent $PSCommandPath
Set-Location $repoRoot

Require-Command kubectl
Require-Command ssh
Require-Command scp
Require-Command docker
Require-Command make

if (-not $SshKeyPath) {
  $localKey = Join-Path $repoRoot "sshKey"
  if (Test-Path $localKey) {
    $SshKeyPath = $localKey
  }
}
if (-not $Image) {
  if ($SkipBuild -or $SkipImageImport) {
    $Image = "dune-admin:local"
  } else {
    $Image = "dune-admin:local-$(Get-Date -Format 'yyyyMMddHHmmss')"
  }
}
if ($SshKeyPath -and -not (Test-Path $SshKeyPath)) {
  throw "SSH key not found: $SshKeyPath"
}
if ($SshKeyPath) {
  Write-Host "Using SSH key: $SshKeyPath (fallback to password enabled)"
} else {
  Write-Host "No SSH key provided/found; using password auth (or agent) for SSH."
}
$sshOpts = Get-SshOptionArgs -KeyPath $SshKeyPath

if (-not $SkipKubeconfig) {
  Invoke-Step "Pulling kubeconfig from $VmUser@$VmHost" {
    $dir = Split-Path -Parent $KubeconfigPath
    if (-not (Test-Path $dir)) {
      New-Item -Path $dir -ItemType Directory -Force | Out-Null
    }
    & ssh @sshOpts "$VmUser@$VmHost" "sudo cat /etc/rancher/k3s/k3s.yaml" | Out-File -FilePath $KubeconfigPath -Encoding utf8NoBOM
    $raw = Get-Content -Path $KubeconfigPath -Raw
    $raw.Replace("127.0.0.1", $VmHost) | Set-Content -Path $KubeconfigPath -Encoding utf8NoBOM
  }
}

$env:KUBECONFIG = $KubeconfigPath
Write-Host "Using KUBECONFIG=$($env:KUBECONFIG)"
kubectl get nodes

if (-not $SkipBuild) {
  Invoke-Step "Building image $Image" {
    docker buildx build --platform linux/amd64 -f deploy/Dockerfile -t $Image --load .
  }
}

if (-not $SkipImageImport) {
  Invoke-Step "Importing image into k3s runtime on $VmHost" {
    $tmpTar = Join-Path $env:TEMP "dune-admin-image.tar"
    if (Test-Path $tmpTar) { Remove-Item $tmpTar -Force }
    docker save -o $tmpTar $Image
    & scp @sshOpts $tmpTar "$VmUser@${VmHost}:/tmp/dune-admin-image.tar"
    & ssh @sshOpts "$VmUser@$VmHost" "sudo k3s ctr images import /tmp/dune-admin-image.tar && rm -f /tmp/dune-admin-image.tar"
    Remove-Item $tmpTar -Force
  }
}

Invoke-Step "Rendering manifest" {
  make render-k8s
}

if (-not (Test-Path $Manifest)) {
  throw "Manifest not found: $Manifest"
}

$manifestText = Get-Content -Path $Manifest -Raw
$patched = [regex]::Replace($manifestText, '(?m)^(\s*image:\s*).*$', "`$1$Image", 1)
if ($patched -eq $manifestText) {
  throw "No image: field found to patch in manifest"
}

$dbHostOverride = ""
$controlNsMatch = [regex]::Match($patched, '(?m)^\s*control_namespace:\s*"?([^\s"]+)"?\s*$')
if ($controlNsMatch.Success) {
  $controlNs = $controlNsMatch.Groups[1].Value
  $svcRows = kubectl -n $controlNs get svc -o jsonpath='{range .items[*]}{.metadata.name}{"`t"}{range .spec.ports[*]}{.port}{" "}{end}{"`n"}{end}' 2>$null
  $dbSvcRow = $svcRows | Where-Object { $_ -match '(^|[ \t])15432([ \t]|$)' } | Select-Object -First 1
  if ($dbSvcRow) {
    $dbSvc = ($dbSvcRow -split '\s+')[0]
    if ($dbSvc) {
      $dbHostOverride = "$dbSvc.$controlNs.svc.cluster.local"
      Write-Host "Using in-cluster DB host: $dbHostOverride"
    }
  }
}

$patched = [regex]::Replace($patched, '(?m)^(\s*CONTROL:\s*).*$', '${1}"local"')
$patched = [regex]::Replace($patched, '(?m)^\s*cmd_(status|start|stop|restart):\s*.*\r?\n', '')
$patched = [regex]::Replace($patched, '(?m)^(\s*control:\s*).*$', '${1}local')
if ($dbHostOverride) {
  $patched = [regex]::Replace($patched, '(?m)^(\s*DB_HOST:\s*).*$', '${1}"' + $dbHostOverride + '"')
  $patched = [regex]::Replace($patched, '(?m)^(\s*db_host:\s*).*$', '${1}' + $dbHostOverride)
}
$patched = [regex]::Replace($patched, '(?m)^\s*ssh_host:\s*.*\r?\n', '')
$patched = [regex]::Replace($patched, '(?m)^\s*ssh_user:\s*.*\r?\n', '')
$patched = [regex]::Replace($patched, '(?m)^\s*ssh_key:\s*.*\r?\n', '')
$patched = [regex]::Replace($patched, '(?m)^(\s*MARKET_BOT_ENABLED:\s*).*$', '${1}"true"')
$patched = [regex]::Replace($patched, '(?m)^(\s*market_bot_enabled:\s*).*$', '${1}true')
$patched = [regex]::Replace($patched, '(?m)^(\s*market_bot_item_data:\s*).*$', '${1}/app/item-data.json')
$patched = [regex]::Replace($patched, '(?m)^(\s*market_bot_cache_db:\s*).*$', '${1}/data/market-bot-cache.db')
$patched | Set-Content -Path $Manifest -Encoding utf8NoBOM

Invoke-Step "Applying manifest" {
  kubectl apply -f $Manifest
  if ($controlNsMatch.Success) {
    $rbac = @"
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dune-admin-runtime
  namespace: $controlNs
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log", "services", "endpoints", "persistentvolumeclaims"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create", "get"]
  - apiGroups: ["igw.funcom.com"]
    resources: ["battlegroups", "serverstats"]
    verbs: ["get", "list", "watch", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dune-admin-runtime
  namespace: $controlNs
subjects:
  - kind: ServiceAccount
    name: default
    namespace: $Namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dune-admin-runtime
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dune-admin-operators-logs
  namespace: funcom-operators
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dune-admin-operators-logs
  namespace: funcom-operators
subjects:
  - kind: ServiceAccount
    name: default
    namespace: $Namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dune-admin-operators-logs
"@
    $rbac | kubectl apply -f -
  }
  kubectl -n $Namespace rollout restart deploy/dune-admin
  kubectl -n $Namespace rollout status deploy/dune-admin
  kubectl -n $Namespace get pods,svc
}

Invoke-Step "In-cluster health checks (fast-fail, no pod waits)" {
  $stalePods = kubectl -n $Namespace get pods --no-headers 2>$null | `
    Where-Object { $_ -match '^(curl|curl-check-)' } | `
    ForEach-Object { ($_ -split '\s+')[0] }
  foreach ($p in $stalePods) {
    if ($p) { kubectl -n $Namespace delete pod $p --ignore-not-found | Out-Null }
  }

  $statusPath = "/api/v1/namespaces/$Namespace/services/http:dune-admin:8080/proxy/api/v1/status"
  $botPath = "/api/v1/namespaces/$Namespace/services/http:dune-admin:8080/proxy/api/v1/market-bot/status"
  $bgPath = "/api/v1/namespaces/$Namespace/services/http:dune-admin:8080/proxy/api/v1/battlegroup/status"

  $healthOk = $false
  $lastStatus = ""
  $lastBot = ""
  $lastBg = ""
  for ($i = 1; $i -le 30; $i++) {
    $lastStatus = (kubectl --request-timeout=5s get --raw $statusPath 2>$null)
    $lastBot = (kubectl --request-timeout=5s get --raw $botPath 2>$null)
    $lastBg = (kubectl --request-timeout=5s get --raw $bgPath 2>$null)

    $botOk = $lastBot -match '"enabled":true'
    $bgOk = $lastBg -notmatch "does not support GetStatus"
    if ($lastStatus -and $botOk -and $bgOk) {
      Write-Host $lastBot
      $healthOk = $true
      break
    }
    if (($i % 5) -eq 0) {
      Write-Host "Health check retry $i/30..."
    }
    Start-Sleep -Seconds 1
  }
  if (-not $healthOk) {
    throw "health check failed: API or embedded market-bot not ready`nlast /api/v1/status: $lastStatus`nlast /api/v1/market-bot/status: $lastBot`nlast /api/v1/battlegroup/status: $lastBg"
  }
}

if ($PortForward -and -not $NoPortForward) {
  Write-Host "Opening API port-forward at http://127.0.0.1:8080 ..."
  kubectl -n $Namespace port-forward svc/dune-admin 8080:8080
} else {
  Write-Host "Deploy complete. Run ./listen.ps1 to open API port-forward."
}
