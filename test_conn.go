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
    
    fmt.Println("✅ Подключение к базе GO успешно!")
}
