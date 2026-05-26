//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "subscription-system/config"
    "subscription-system/database"
)

func main() {
    // Загружаем конфиг
    cfg := config.Load()
    
    // Подключаемся к БД
    if err := database.InitDB(cfg); err != nil {
        log.Fatalf("❌ Ошибка подключения: %v", err)
    }
    defer database.CloseDB()

    ctx := context.Background()

    // Создаем таблицу customers
    _, err := database.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS customers (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            phone VARCHAR(50),
            email VARCHAR(255),
            company VARCHAR(255),
            created_by VARCHAR(100),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания customers: %v", err)
    } else {
        fmt.Println("✅ Таблица customers создана")
    }

    // Создаем таблицу deals
    _, err = database.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS deals (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            customer_id INTEGER REFERENCES customers(id) ON DELETE SET NULL,
            status VARCHAR(50) DEFAULT 'new',
            amount DECIMAL(10,2),
            created_by VARCHAR(100),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания deals: %v", err)
    } else {
        fmt.Println("✅ Таблица deals создана")
    }

    // Создаем таблицу teams
    _, err = database.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS teams (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            description TEXT,
            created_by VARCHAR(100),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания teams: %v", err)
    } else {
        fmt.Println("✅ Таблица teams создана")
    }

    // Создаем таблицу team_members
    _, err = database.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS team_members (
            id SERIAL PRIMARY KEY,
            team_id INTEGER REFERENCES teams(id) ON DELETE CASCADE,
            user_name VARCHAR(100),
            role VARCHAR(50) DEFAULT 'member',
            joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания team_members: %v", err)
    } else {
        fmt.Println("✅ Таблица team_members создана")
    }

    // Добавляем тестовые данные
    _, err = database.Pool.Exec(ctx, `
        INSERT INTO customers (name, created_by) 
        VALUES ('Тестовый клиент', 'system')
        ON CONFLICT (id) DO NOTHING
    `)
    if err == nil {
        fmt.Println("✅ Тестовый клиент добавлен")
    }

    _, err = database.Pool.Exec(ctx, `
        INSERT INTO deals (name, status, created_by) 
        VALUES ('Тестовая сделка', 'new', 'system')
        ON CONFLICT (id) DO NOTHING
    `)
    if err == nil {
        fmt.Println("✅ Тестовая сделка добавлена")
    }

    fmt.Println("\n✅ Все таблицы успешно созданы!")
    fmt.Println("📊 Таблицы: customers, deals, teams, team_members")
}
