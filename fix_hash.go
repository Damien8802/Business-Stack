//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "github.com/jackc/pgx/v5"
    "golang.org/x/crypto/bcrypt"
)

func main() {
    connStr := "postgres://postgres:6213110@localhost:5432/GO?sslmode=disable"
    
    conn, err := pgx.Connect(context.Background(), connStr)
    if err != nil {
        log.Fatal("Ошибка подключения:", err)
    }
    defer conn.Close(context.Background())
    
    // Генерируем новый хэш
    password := "test123"
    hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        log.Fatal("Hash error:", err)
    }
    
    // Обновляем пароль
    _, err = conn.Exec(context.Background(), `
        UPDATE users 
        SET password_hash = $1, password_changed_at = NOW()
        WHERE email = 'test_204618@example.com'
    `, string(hash))
    
    if err != nil {
        log.Fatal("Update failed:", err)
    }
    
    fmt.Println("✅ Пароль обновлен для test_204618@example.com")
    fmt.Println("Пароль: test123")
}
