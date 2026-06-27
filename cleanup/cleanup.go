package cleanup

import (
    "context"
    "log"
    "time"

    "subscription-system/database"
)

// StartCleanupScheduler - запускает планировщик очистки
func StartCleanupScheduler() {
    go func() {
        ticker := time.NewTicker(336 * time.Hour) // 14 дней
        defer ticker.Stop()
        
        log.Println("🔄 Планировщик очистки запущен (каждые 2 недели)")
        
        time.Sleep(1 * time.Minute)
        runCleanup()
        
        for range ticker.C {
            runCleanup()
        }
    }()
}

func runCleanup() {
    ctx := context.Background()
    
    // 1. Удаляем историю входов старше 14 дней (ИСПРАВЛЕНО!)
    result, err := database.Pool.Exec(ctx, `
        DELETE FROM login_history 
        WHERE login_time < NOW() - INTERVAL '14 days'
    `)
    if err != nil {
        log.Printf("⚠️ Ошибка очистки login_history: %v", err)
    } else if rows := result.RowsAffected(); rows > 0 {
        log.Printf("🧹 Очистка login_history: удалено %d записей (старше 14 дней)", rows)
    }
    
    // 2. Удаляем истекшие сессии (старше 7 дней после истечения)
    result, err = database.Pool.Exec(ctx, `
        DELETE FROM user_sessions 
        WHERE expires_at < NOW() - INTERVAL '7 days'
    `)
    if err != nil {
        log.Printf("⚠️ Ошибка очистки user_sessions: %v", err)
    } else if rows := result.RowsAffected(); rows > 0 {
        log.Printf("🧹 Очистка user_sessions: удалено %d записей (истекшие сессии)", rows)
    }
    
    // 3. Удаляем старые доверенные устройства (просроченные)
    result, err = database.Pool.Exec(ctx, `
        DELETE FROM trusted_devices 
        WHERE expires_at < NOW() - INTERVAL '30 days'
    `)
    if err != nil {
        log.Printf("⚠️ Ошибка очистки trusted_devices: %v", err)
    } else if rows := result.RowsAffected(); rows > 0 {
        log.Printf("🧹 Очистка trusted_devices: удалено %d записей (просроченные устройства)", rows)
    }
    
    log.Println("✅ Очистка БД завершена")
}