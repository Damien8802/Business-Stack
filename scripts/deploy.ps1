# deploy.ps1 - Deploy Business Stack to production server
param(
    [Parameter(Mandatory=$true)]
    [string]$ServerIP,
    
    [string]$Username = "root",
    [string]$KeyPath = "$env:USERPROFILE\.ssh\id_rsa"
)

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  🚀 Business Stack 3.0 Production Deploy"
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 1. Build for Linux
Write-Host "1. 🔨 Building for Linux..." -ForegroundColor Green
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

go build -o Business Stack-linux -ldflags="-s -w" main.go

if (-not $?) {
    Write-Host "❌ Build failed!" -ForegroundColor Red
    exit 1
}

Write-Host "✅ Build successful: Business Stack-linux" -ForegroundColor Green

# 2. Prepare files
Write-Host "`n2. 📦 Preparing files..." -ForegroundColor Green

# Create deployment package
$deployDir = "deploy-package"
New-Item -ItemType Directory -Path $deployDir -Force | Out-Null

# Copy files
Copy-Item "Business Stack-linux" -Destination "$deployDir/Business Stack"
Copy-Item ".env.production" -Destination "$deployDir/.env"
Copy-Item "templates" -Destination $deployDir -Recurse
Copy-Item "static" -Destination $deployDir -Recurse
Copy-Item "frontend" -Destination $deployDir -Recurse

Write-Host "✅ Package created: $deployDir" -ForegroundColor Green

# 3. Upload to server
Write-Host "`n3. 📤 Uploading to server $ServerIP..." -ForegroundColor Green

# Check SSH key
if (-not (Test-Path $KeyPath)) {
    Write-Host "❌ SSH key not found: $KeyPath" -ForegroundColor Red
    Write-Host "Generate SSH key: ssh-keygen -t rsa -b 4096" -ForegroundColor Yellow
    exit 1
}

# Create upload script
$uploadScript = @"
#!/bin/bash
set -e

echo "📁 Creating directories..."
sudo mkdir -p /opt/Business Stack/{bin,logs,uploads,backups,templates,static,frontend}

echo "📦 Copying files..."
sudo cp -r /tmp/deploy-package/* /opt/Business Stack/
sudo chmod +x /opt/Business Stack/Business Stack
sudo chown -R Business Stack:Business Stack /opt/Business Stack

echo "⚙️  Restarting service..."
sudo systemctl daemon-reload
sudo systemctl restart Business Stack

echo "✅ Deployment completed!"
echo ""
echo "🌐 Application: http://$ServerIP:8080"
"@

$uploadScript | Out-File -FilePath "$deployDir/deploy.sh" -Encoding UTF8

# Upload package
Write-Host "  • Uploading package..." -ForegroundColor Gray
scp -i $KeyPath -r $deployDir ${Username}@${ServerIP}:/tmp/

# Execute deploy script
Write-Host "  • Executing deploy script..." -ForegroundColor Gray
ssh -i $KeyPath ${Username}@${ServerIP} "bash /tmp/deploy-package/deploy.sh"

# 4. Cleanup
Write-Host "`n4. 🧹 Cleaning up..." -ForegroundColor Green
Remove-Item -Path $deployDir -Recurse -Force
Remove-Item -Path "Business Stack-linux" -Force

Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "  🎉 DEPLOYMENT COMPLETED!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "🌐 Your application is now live at:" -ForegroundColor Cyan
Write-Host "   http://$ServerIP:8080" -ForegroundColor White -BackgroundColor DarkBlue
Write-Host ""
Write-Host "🔧 Management commands:" -ForegroundColor Gray
Write-Host "   ssh -i $KeyPath ${Username}@${ServerIP}" -ForegroundColor Gray
Write-Host "   sudo systemctl status Business Stack" -ForegroundColor Gray
Write-Host "   sudo journalctl -u Business Stack -f" -ForegroundColor Gray
Write-Host "   sudo systemctl restart Business Stack" -ForegroundColor Gray
Write-Host ""