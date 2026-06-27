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

 // ДОБАВЛЯЕМ ПОЛЯ В ТАБЛИЦУ users ДЛЯ СКИДКИ
    log.Println("Добавление полей скидки в таблицу users...")
    
    _, err = database.Pool.Exec(ctx, `
        DO $$ 
        BEGIN
            IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                           WHERE table_name='users' AND column_name='twofa_discount_active') THEN
                ALTER TABLE users ADD COLUMN twofa_discount_active BOOLEAN DEFAULT false;
            END IF;
            
            IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                           WHERE table_name='users' AND column_name='twofa_discount_start_date') THEN
                ALTER TABLE users ADD COLUMN twofa_discount_start_date DATE;
            END IF;
            
            IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                           WHERE table_name='users' AND column_name='twofa_discount_end_date') THEN
                ALTER TABLE users ADD COLUMN twofa_discount_end_date DATE;
            END IF;
            
            IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                           WHERE table_name='users' AND column_name='twofa_discount_used') THEN
                ALTER TABLE users ADD COLUMN twofa_discount_used BOOLEAN DEFAULT false;
            END IF;
        END $$;
    `)

    if err != nil {
        log.Fatal("❌ Ошибка добавления полей в users:", err)
    }
    log.Println("✅ Поля скидки в users добавлены")

    // СОЗДАЕМ ТАБЛИЦУ twofa_discount_history
    log.Println("Создание таблицы twofa_discount_history...")
    
    _, err = database.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS twofa_discount_history (
            id SERIAL PRIMARY KEY,
            user_id UUID REFERENCES users(id) ON DELETE CASCADE,
            discount_percent INTEGER DEFAULT 5,
            applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            valid_from DATE,
            valid_to DATE,
            subscription_id INTEGER,
            status VARCHAR(20) DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE INDEX IF NOT EXISTS idx_twofa_discount_user_id ON twofa_discount_history(user_id);
        CREATE INDEX IF NOT EXISTS idx_twofa_discount_status ON twofa_discount_history(status);
        CREATE INDEX IF NOT EXISTS idx_twofa_discount_dates ON twofa_discount_history(valid_from, valid_to);
    `)

    if err != nil {
        log.Fatal("❌ Ошибка создания таблицы twofa_discount_history:", err)
    }
    log.Println("✅ Таблица twofa_discount_history создана")
    
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

