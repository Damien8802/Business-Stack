//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "github.com/jackc/pgx/v5"
)

func main() {
    connStr := "postgres://postgres:6213110@localhost:5432/GO?sslmode=disable"
    
    conn, err := pgx.Connect(context.Background(), connStr)
    if err != nil {
        log.Fatal("Ошибка подключения:", err)
    }
    defer conn.Close(context.Background())
    
    // Находим последнего созданного пользователя
    var email, name, role string
    err = conn.QueryRow(context.Background(), `
        SELECT email, name, role FROM users 
        ORDER BY created_at DESC LIMIT 1
    `).Scan(&email, &name, &role)
    
    if err != nil {
        log.Fatal("Query failed:", err)
    }
    
    fmt.Printf("Последний пользователь:\nEmail: %s\nName: %s\nRole: %s\nПароль: test123\n", email, name, role)
}
