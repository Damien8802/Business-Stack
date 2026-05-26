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
    cfg := config.Load()
    if err := database.InitDB(cfg); err != nil {
        log.Fatal(err)
    }
    defer database.CloseDB()

    rows, err := database.Pool.Query(context.Background(), 
        "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'twofa'")
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    defer rows.Close()

    fmt.Println("=== Таблица twofa ===")
    for rows.Next() {
        var col, typ string
        rows.Scan(&col, &typ)
        fmt.Printf("%s: %s\n", col, typ)
    }
}
