//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "github.com/jackc/pgx/v5"
)

func main() {
    connStr := "postgres://postgres:6213110@localhost:5432/postgres?sslmode=disable"
    
    conn, err := pgx.Connect(context.Background(), connStr)
    if err != nil {
        log.Fatal("Ошибка подключения:", err)
    }
    defer conn.Close(context.Background())
    
    // Создаем новую базу
    _, err = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS GO")
    if err != nil {
        log.Println("Ошибка удаления:", err)
    }
    
    _, err = conn.Exec(context.Background(), "CREATE DATABASE GO OWNER postgres")
    if err != nil {
        log.Fatal("Ошибка создания:", err)
    }
    
    fmt.Println("✅ База GO пересоздана")
}
