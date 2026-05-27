[CmdletBinding()]
param(
  [string]$Namespace = "dune-admin",
  [int]$LocalPort = 8080,
  [int]$RemotePort = 8080
)

$ErrorActionPreference = "Stop"

Write-Host "Opening API port-forward at http://127.0.0.1:$LocalPort ..."
kubectl -n $Namespace port-forward svc/dune-admin "$LocalPort`:$RemotePort"
