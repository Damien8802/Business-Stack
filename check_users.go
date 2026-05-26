//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "github.com/jackc/pgx/v5"
)

func main() {
    dbHost := "localhost"
    dbPort := "5432"
    dbUser := "postgres"
    dbPassword := "6213110"
    dbName := "GO"
    
    connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", 
        dbUser, dbPassword, dbHost, dbPort, dbName)
    
    fmt.Println("Подключение к базе...")
    conn, err := pgx.Connect(context.Background(), connStr)
    if err != nil {
        log.Fatal("Ошибка подключения:", err)
    }
    defer conn.Close(context.Background())
    fmt.Println("✅ Подключено!")
    
    // Проверяем структуру таблицы users
    rows, err := conn.Query(context.Background(), `
        SELECT column_name, data_type, is_nullable 
        FROM information_schema.columns 
        WHERE table_name = 'users'
        ORDER BY ordinal_position
    `)
    if err != nil {
        log.Fatal("Query failed:", err)
    }
    defer rows.Close()
    
    fmt.Println("\n=== СТРУКТУРА ТАБЛИЦЫ USERS ===\n")
    for rows.Next() {
        var column, dataType, nullable string
        rows.Scan(&column, &dataType, &nullable)
        fmt.Printf("%s (%s) - nullable: %s\n", column, dataType, nullable)
    }
    
    // Проверяем существующих пользователей
    fmt.Println("\n=== СУЩЕСТВУЮЩИЕ ПОЛЬЗОВАТЕЛИ ===\n")
    userRows, err := conn.Query(context.Background(), `
        SELECT id, email, name, role FROM users LIMIT 5
    `)
    if err != nil {
        log.Fatal("Query users failed:", err)
    }
    defer userRows.Close()
    
    for userRows.Next() {
        var id, email, name, role string
        userRows.Scan(&id, &email, &name, &role)
        fmt.Printf("ID: %s\nEmail: %s\nName: %s\nRole: %s\n---\n", id, email, name, role)
    }
}
