# cheat-sheet.ps1 - Шпаргалка по командам Business Stack WSL2

Write-Host "📚 Шпаргалка по Business Stack 3.0 WSL2" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

Write-Host "🚀 ЗАПУСК:" -ForegroundColor Green
Write-Host "  .\run-wsl.ps1                    # Продакшен режим" -ForegroundColor Gray
Write-Host "  .\run-wsl.ps1 -Dev              # Режим разработки" -ForegroundColor Gray
Write-Host "  .\run-wsl.ps1 -Monitor          # С мониторингом" -ForegroundColor Gray
Write-Host "  .\run-wsl.ps1 -Benchmark        # Бенчмарк тесты" -ForegroundColor Gray
Write-Host "  .\run-wsl.ps1 -Build            # Пересобрать и запустить" -ForegroundColor Gray
Write-Host ""

Write-Host "🐳 DOCKER КОМАНДЫ:" -ForegroundColor Green
Write-Host "  docker-compose up -d            # Запуск в фоне" -ForegroundColor Gray
Write-Host "  docker-compose down             # Остановка" -ForegroundColor Gray
Write-Host "  docker-compose logs -f          # Логи в реальном времени" -ForegroundColor Gray
Write-Host "  docker-compose restart          # Перезапуск" -ForegroundColor Gray
Write-Host "  docker-compose ps               # Статус контейнеров" -ForegroundColor Gray
Write-Host ""

Write-Host "📊 МОНИТОРИНГ:" -ForegroundColor Green
Write-Host "  docker stats                    # Использование ресурсов" -ForegroundColor Gray
Write-Host "  docker top Business Stack-wsl2         # Процессы в контейнере" -ForegroundColor Gray
Write-Host "  docker exec -it Business Stack-wsl2 sh # Войти в контейнер" -ForegroundColor Gray
Write-Host "  curl http://localhost:8080/health # Проверить здоровье" -ForegroundColor Gray
Write-Host ""

Write-Host "🔧 РАЗРАБОТКА:" -ForegroundColor Green
Write-Host "  go run main.go                  # Быстрый запуск без Docker" -ForegroundColor Gray
Write-Host "  go build -o Business Stack.exe         # Сборка бинарника" -ForegroundColor Gray
Write-Host "  go test ./...                   # Запуск тестов" -ForegroundColor Gray
Write-Host "  go mod tidy                     # Обновить зависимости" -ForegroundColor Gray
Write-Host ""

Write-Host "⚡ WSL2 ОПТИМИЗАЦИЯ:" -ForegroundColor Green
Write-Host "  .\wsl2-setup.ps1               # Настройка WSL2" -ForegroundColor Gray
Write-Host "  .\restart-wsl.ps1              # Перезапуск WSL2" -ForegroundColor Gray
Write-Host "  .\high-performance.ps1         # Включить высокую производительность" -ForegroundColor Gray
Write-Host "  wsl --shutdown                 # Полная остановка WSL2" -ForegroundColor Gray
Write-Host "  wsl -l -v                      # Список дистрибутивов WSL" -ForegroundColor Gray
Write-Host ""

Write-Host "📁 ФАЙЛЫ ПРОЕКТА:" -ForegroundColor Green
Write-Host "  main.go                        # Основной файл приложения" -ForegroundColor Gray
Write-Host "  Dockerfile                     # Конфигурация Docker" -ForegroundColor Gray
Write-Host "  docker-compose.yml            # Docker Compose" -ForegroundColor Gray
Write-Host "  run-wsl.ps1                   # Основной скрипт запуска" -ForegroundColor Gray
Write-Host "  templates/                     # HTML шаблоны (67 файлов)" -ForegroundColor Gray
Write-Host "  static/                        # Статические файлы" -ForegroundColor Gray
Write-Host "  frontend/                      # Фронтенд файлы" -ForegroundColor Gray
Write-Host "  logs/                          # Логи приложения" -ForegroundColor Gray
Write-Host ""

Write-Host "🌐 ВЕБ-ИНТЕРФЕЙС:" -ForegroundColor Green
Write-Host "  http://localhost:8080          # Главная страница" -ForegroundColor Blue
Write-Host "  http://localhost:8080/health   # Health Check" -ForegroundColor Blue
Write-Host "  http://localhost:8080/dashboard # Дашборд" -ForegroundColor Blue
Write-Host "  http://localhost:8080/admin    # Админка" -ForegroundColor Blue
Write-Host "  http://localhost:8080/metrics  # Метрики (если включены)" -ForegroundColor Blue
Write-Host ""

Write-Host "🎯 БЫСТРЫЕ АЛИАСЫ (загрузите: . .\alias.ps1):" -ForegroundColor Green
Write-Host "  Start-Business Stack                  # Запустить Business Stack" -ForegroundColor Gray
Write-Host "  Stop-Business Stack                   # Остановить Business Stack" -ForegroundColor Gray
Write-Host "  Show-Business Stack-Logs              # Показать логи" -ForegroundColor Gray
Write-Host "  Test-Business Stack-Health            # Проверить здоровье" -ForegroundColor Gray
Write-Host "  Open-Business Stack                   # Открыть в браузере" -ForegroundColor Gray
Write-Host ""

Write-Host "✅ Business Stack 3.0 готов к работе в WSL2!" -ForegroundColor Cyan
Write-Host "   Для начала работы: .\run-wsl.ps1" -ForegroundColor Yellow
