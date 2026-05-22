# start-dev.ps1 – запуск всех компонентов Business Stack

Write-Host "🚀 Запуск всех компонентов Business Stack..." -ForegroundColor Cyan

# 1. Запуск основного сервера (Go)
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd C:\Projects\subscription-clean-WORKS; go run main.go" -WindowStyle Normal

Start-Sleep -Seconds 2

# 2. Запуск Telegram-бота
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd C:\Projects\subscription-clean-WORKS\telegram-bot; go run main.go" -WindowStyle Normal

# 3. Запуск сервера Mini App (http-server)
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd C:\Projects\subscription-clean-WORKS\telegram-mini-app; npx http-server -p 3000" -WindowStyle Normal

Write-Host "✅ Все компоненты запущены. Можете сворачивать это окно." -ForegroundColor Green
