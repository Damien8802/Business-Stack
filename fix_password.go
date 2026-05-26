//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "subscription-system/database"
    "subscription-system/config"
    "golang.org/x/crypto/bcrypt"
)

func main() {
    cfg := config.Load()
    
    err := database.InitDB(cfg)
    if err != nil {
        log.Fatal("DB init failed:", err)
    }
    defer database.CloseDB()
    
    // Получаем хэш из БД
    var dbHash string
    err = database.Pool.QueryRow(context.Background(), `
        SELECT password_hash FROM users WHERE email = 'admin@example.com'
    `).Scan(&dbHash)
    if err != nil {
        log.Fatal("User not found:", err)
    }
    
    fmt.Println("Hash in DB:", dbHash)
    
    // Проверяем пароль
    password := "admin123"
    err = bcrypt.CompareHashAndPassword([]byte(dbHash), []byte(password))
    if err == nil {
        fmt.Println("✅ Пароль верный!")
    } else {
        fmt.Println("❌ Пароль неверный:", err)
    }
    
    // Сгенерируем новый хэш и проверим
    newHash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    fmt.Println("\nNew generated hash:", string(newHash))
    
    // Обновим пароль в БД
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE users SET password_hash = $1, password_changed_at = NOW()
        WHERE email = 'admin@example.com'
    `, string(newHash))
    if err != nil {
        log.Fatal("Update failed:", err)
    }
    
    fmt.Println("\n✅ Пароль обновлен!")
    fmt.Println("Теперь попробуйте войти с паролем: admin123")
}
