//go:build ignore

package main

import (
    "context"
    "log"
    "subscription-system/config"
    "subscription-system/database"
)

func main() {
    cfg := config.Load()
    
    log.Println("Подключение к базе данных...")
    if err := database.InitDB(cfg); err != nil {
        log.Fatal("❌ Ошибка подключения к БД:", err)
    }
    defer database.CloseDB()

    ctx := context.Background()
    
    log.Println("Выполнение миграции таблицы twofa...")
    
    // Добавляем колонки
    _, err := database.Pool.Exec(ctx, `
        DO $$ 
        BEGIN
            BEGIN
                ALTER TABLE twofa ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP DEFAULT NOW() + INTERVAL ''10 minutes'';
                RAISE NOTICE ''Колонка expires_at добавлена'';
            EXCEPTION
                WHEN duplicate_column THEN 
                    RAISE NOTICE ''Колонка expires_at уже существует'';
            END;
            
            BEGIN
                ALTER TABLE twofa ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT NOW();
                RAISE NOTICE ''Колонка created_at добавлена'';
            EXCEPTION
                WHEN duplicate_column THEN 
                    RAISE NOTICE ''Колонка created_at уже существует'';
            END;
            
            BEGIN
                ALTER TABLE twofa ADD COLUMN IF NOT EXISTS used BOOLEAN DEFAULT FALSE;
                RAISE NOTICE ''Колонка used добавлена'';
            EXCEPTION
                WHEN duplicate_column THEN 
                    RAISE NOTICE ''Колонка used уже существует'';
            END;
        END $$;
    `)
    
    if err != nil {
        log.Fatal("❌ Ошибка миграции:", err)
    }
    
    log.Println("✅ Таблица twofa успешно обновлена!")
    
    // Проверяем структуру таблицы
    rows, err := database.Pool.Query(ctx, `
        SELECT column_name, data_type 
        FROM information_schema.columns 
        WHERE table_name = ''twofa''
        ORDER BY ordinal_position
    `)
    if err != nil {
        log.Println("Не удалось проверить структуру:", err)
        return
    }
    defer rows.Close()
    
    log.Println("Структура таблицы twofa:")
    for rows.Next() {
        var columnName, dataType string
        rows.Scan(&columnName, &dataType)
        log.Printf("  - %s (%s)", columnName, dataType)
    }
}
