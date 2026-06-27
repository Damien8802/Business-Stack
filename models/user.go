package models

import (
    "context"
    "time"

    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"
    "subscription-system/database"
)

type User struct {
    ID               uuid.UUID `json:"id"`
    Email            string    `json:"email"`
    PasswordHash     string    `json:"-"`
    Login            string    `json:"login"`
    Name             string    `json:"name"`
    Role             string    `json:"role"`
    Phone            string    `json:"phone"`
    OrganizationName string    `json:"organization_name"`
    OrganizationInn  string    `json:"organization_inn"`
    EmailVerified    bool      `json:"email_verified"`
    TelegramID       *int64    `json:"telegram_id,omitempty"`
    PasswordChangedAt time.Time `json:"password_changed_at"`
    TenantID          string    `json:"tenant_id"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
}

func GetUserByEmail(email string) (*User, error) {
    var user User
    err := database.Pool.QueryRow(context.Background(), `
        SELECT id, email, password_hash, name, role, 
               COALESCE(phone, '') as phone,
               COALESCE(organization_name, '') as organization_name,
               COALESCE(organization_inn, '') as organization_inn,
               email_verified, telegram_id, password_changed_at, created_at, updated_at
        FROM users WHERE email = $1
    `, email).Scan(
        &user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
        &user.Phone, &user.OrganizationName, &user.OrganizationInn,
        &user.EmailVerified, &user.TelegramID, &user.PasswordChangedAt,
        &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &user, nil
}

func GetUserByID(id string) (*User, error) {
    var user User
    err := database.Pool.QueryRow(context.Background(), `
        SELECT id, email, password_hash, name, role, 
               COALESCE(phone, '') as phone,
               COALESCE(organization_name, '') as organization_name,
               COALESCE(organization_inn, '') as organization_inn,
               email_verified, telegram_id, password_changed_at, created_at, updated_at
        FROM users WHERE id = $1
    `, id).Scan(
        &user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
        &user.Phone, &user.OrganizationName, &user.OrganizationInn,
        &user.EmailVerified, &user.TelegramID, &user.PasswordChangedAt,
        &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &user, nil
}

func CreateUser(email, passwordHash, name, tenantID string) (*User, error) {
    var user User
    defaultRole := "client"

    err := database.Pool.QueryRow(context.Background(), `
        INSERT INTO users (email, password_hash, name, role, tenant_id, password_changed_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
        RETURNING id, email, password_hash, name, role, 
                  COALESCE(phone, '') as phone,
                  COALESCE(organization_name, '') as organization_name,
                  COALESCE(organization_inn, '') as organization_inn,
                  email_verified, telegram_id, password_changed_at, created_at, updated_at
    `, email, passwordHash, name, defaultRole, tenantID).Scan(
        &user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
        &user.Phone, &user.OrganizationName, &user.OrganizationInn,
        &user.EmailVerified, &user.TelegramID, &user.PasswordChangedAt,
        &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    return &user, nil
}

func UpdateUserPassword(userID string, newPasswordHash string) error {
    _, err := database.Pool.Exec(context.Background(), `
        UPDATE users SET password_hash = $1, password_changed_at = NOW(), updated_at = NOW()
        WHERE id = $2
    `, newPasswordHash, userID)
    return err
}

func UpdateUserProfile(userID string, name string) error {
    _, err := database.Pool.Exec(context.Background(), `
        UPDATE users SET name = $1, updated_at = NOW()
        WHERE id = $2
    `, name, userID)
    return err
}

func CheckPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}

func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func UpdateUser(userID string, updates map[string]interface{}) error {
    ctx := context.Background()
    if len(updates) == 0 {
        return nil
    }
    
    query := "UPDATE users SET updated_at = NOW()"
    args := []interface{}{}
    i := 1
    for key, value := range updates {
        query += ", " + key + " = $" + string(rune(48+i))
        args = append(args, value)
        i++
    }
    query += " WHERE id = $" + string(rune(48+i))
    args = append(args, userID)
    
    _, err := database.Pool.Exec(ctx, query, args...)
    return err
}

func UpdatePassword(userID string, newPasswordHash string) error {
    _, err := database.Pool.Exec(context.Background(), `
        UPDATE users SET password_hash = $1, password_changed_at = NOW(), updated_at = NOW()
        WHERE id = $2
    `, newPasswordHash, userID)
    return err
}