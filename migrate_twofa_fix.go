//go:build ignore

package main

import (
    "context"
    "log"
    "os"
    "github.com/jackc/pgx/v5"
)

func main() {
    // Берем параметры из переменных окружения
    dbHost := os.Getenv("DB_HOST")
    if dbHost == "" {
        dbHost = "localhost"
    }
    dbPort := os.Getenv("DB_PORT")
    if dbPort == "" {
        dbPort = "5432"
    }
    dbUser := os.Getenv("DB_USER")
    if dbUser == "" {
        dbUser = "postgres"
    }
    dbPassword := os.Getenv("DB_PASSWORD")
    dbName := os.Getenv("DB_NAME")
    if dbName == "" {
        dbName = "GO"
    }
    
    connStr := "postgres://" + dbUser + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName + "?sslmode=disable"
    
    log.Println("Подключение к базе данных:", dbName)
    
    conn, err := pgx.Connect(context.Background(), connStr)
    if err != nil {
        log.Fatal("❌ Ошибка подключения к БД:", err)
    }
    defer conn.Close(context.Background())
    
    log.Println("Выполнение миграции таблицы twofa...")
    
    // Добавляем колонки
    _, err = conn.Exec(context.Background(), `
        ALTER TABLE twofa ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP DEFAULT NOW() + INTERVAL '10 minutes';
        ALTER TABLE twofa ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT NOW();
        ALTER TABLE twofa ADD COLUMN IF NOT EXISTS used BOOLEAN DEFAULT FALSE;
    `)
    
    if err != nil {
        log.Fatal("❌ Ошибка миграции:", err)
    }
    
    log.Println("✅ Таблица twofa успешно обновлена!")
}
