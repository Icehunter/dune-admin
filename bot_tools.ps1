param (
    [string]$BaseUrl = "http://localhost:8080"
)

function Get-BotConfig {
    try {
        $response = Invoke-RestMethod -Uri "$BaseUrl/api/v1/landsraad/bot/config" -Method Get
        $response | ConvertTo-Json -Depth 5
    } catch {
        Write-Error "Failed to fetch bot config: $_"
    }
}

function Set-BotConfig {
    param (
        [Parameter(Mandatory=$true)]
        [string]$JsonPayload
    )
    try {
        $response = Invoke-RestMethod -Uri "$BaseUrl/api/v1/landsraad/bot/config" -Method Put -Body $JsonPayload -ContentType "application/json"
        Write-Host "Config Updated Successfully!" -ForegroundColor Green
        $response | ConvertTo-Json -Depth 5
    } catch {
        Write-Error "Failed to update bot config: $_"
    }
}

function New-NPCGuild {
    param (
        [Parameter(Mandatory=$true)]
        [string]$Name,
        [string]$Description = "NPC Guild",
        [int]$FactionId = 1 # 1 for Atreides, 2 for Harkonnen
    )
    $payload = @{
        name = $Name
        description = $Description
        faction_id = $FactionId
    } | ConvertTo-Json
    try {
        $response = Invoke-RestMethod -Uri "$BaseUrl/api/v1/guilds" -Method Post -Body $payload -ContentType "application/json"
        Write-Host "Guild Created Successfully!" -ForegroundColor Green
        $response | ConvertTo-Json
    } catch {
        Write-Error "Failed to create guild: $_"
    }
}

function Enable-Bot {
    $cfg = Invoke-RestMethod -Uri "$BaseUrl/api/v1/landsraad/bot/config" -Method Get
    $cfg.enabled = $true
    $json = $cfg | ConvertTo-Json -Depth 5
    Set-BotConfig -JsonPayload $json
}

function Disable-Bot {
    $cfg = Invoke-RestMethod -Uri "$BaseUrl/api/v1/landsraad/bot/config" -Method Get
    $cfg.enabled = $false
    $json = $cfg | ConvertTo-Json -Depth 5
    Set-BotConfig -JsonPayload $json
}

Write-Host "Landsraad Bot Local Dev Tools Loaded!" -ForegroundColor Cyan
Write-Host "Available Commands:"
Write-Host "  Get-BotConfig"
Write-Host "  Set-BotConfig -JsonPayload '{...}'"
Write-Host "  New-NPCGuild -Name 'House Atreides NPCs' -FactionId 1"
Write-Host "  Enable-Bot"
Write-Host "  Disable-Bot"
Write-Host ""
Write-Host "Note: Make sure dune-admin.exe is running on $BaseUrl" -ForegroundColor Yellow
