package handlers

import (
    "context"
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log"  
    "net/http"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "golang.org/x/crypto/bcrypt" 
    
    "subscription-system/database"
    "subscription-system/utils"   
)
// OAuth2 клиент
type OAuthClient struct {
    ID           uuid.UUID `json:"id"`
    ClientID     string    `json:"client_id"`
    ClientSecret string    `json:"-"`
    ClientName   string    `json:"client_name"`
    ClientURI    string    `json:"client_uri"`
    RedirectURIs []string  `json:"redirect_uris"`
    Grants       []string  `json:"grants"`
    Scopes       []string  `json:"scopes"`
    Confidential bool      `json:"confidential"`
    Active       bool      `json:"active"`
    CreatedAt    time.Time `json:"created_at"`
}

// OpenID Connect конфигурация
type OIDCConfig struct {
    Issuer                           string   `json:"issuer"`
    AuthorizationEndpoint            string   `json:"authorization_endpoint"`
    TokenEndpoint                    string   `json:"token_endpoint"`
    UserinfoEndpoint                 string   `json:"userinfo_endpoint"`
    JWKSUri                          string   `json:"jwks_uri"`
    ResponseTypesSupported           []string `json:"response_types_supported"`
    SubjectTypesSupported            []string `json:"subject_types_supported"`
    IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
    ScopesSupported                  []string `json:"scopes_supported"`
    ClaimsSupported                  []string `json:"claims_supported"`
}

// OpenID Connect JWK
type JWK struct {
    Kty string `json:"kty"`
    Kid string `json:"kid"`
    Use string `json:"use"`
    Alg string `json:"alg"`
    N   string `json:"n"`
    E   string `json:"e"`
}

// Страница управления OAuth клиентами (админка)
func OAuthClientsPageHandler(c *gin.Context) {
    log.Println("🔍 OAuthClientsPageHandler START")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, client_id, name, redirect_uris, scopes, created_at, status
        FROM oauth_clients
        WHERE status = 'active' OR status IS NULL
        ORDER BY created_at DESC
    `)
    if err != nil {
        log.Printf("❌ Query error: %v", err)
        c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Database error: " + err.Error()})
        return
    }
    defer rows.Close()
    log.Println("✅ Query successful")

    var clients []map[string]interface{}
    for rows.Next() {
        var id, clientID, name, scopes, status string
        var redirectURIs interface{}
        var createdAt time.Time
        
        err := rows.Scan(&id, &clientID, &name, &redirectURIs, &scopes, &createdAt, &status)
        if err != nil {
            log.Printf("⚠️ Scan error: %v", err)
            continue
        }
        
        clients = append(clients, gin.H{
            "id":           id,
            "client_id":    clientID,
            "client_name":  name,
            "redirect_uris": redirectURIs,
            "scopes":       scopes,
            "active":       status != "inactive",
            "created_at":   createdAt,
        })
    }
    
    log.Printf("📊 Found %d clients", len(clients))

    c.HTML(http.StatusOK, "oauth-clients.html", gin.H{
        "clients": clients,
        "title":   "Управление OAuth клиентами",
    })
    log.Println("✅ OAuthClientsPageHandler END")
}
// Создать OAuth клиент
func CreateOAuthClient(c *gin.Context) {
    var req struct {
        ClientName   string   `json:"client_name" binding:"required"`
        ClientURI    string   `json:"client_uri"`
        RedirectURIs []string `json:"redirect_uris" binding:"required"`
        Grants       []string `json:"grants"`
        Scopes       []string `json:"scopes"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Генерируем client_id и client_secret
    clientID := generateRandomString(32)
    clientSecret := generateRandomString(64)
    
    if len(req.Grants) == 0 {
        req.Grants = []string{"authorization_code", "refresh_token"}
    }
    if len(req.Scopes) == 0 {
        req.Scopes = []string{"openid", "profile", "email"}
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO oauth_clients (client_id, client_secret, client_name, client_uri, redirect_uris, grants, scopes)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, clientID, clientSecret, req.ClientName, req.ClientURI, req.RedirectURIs, req.Grants, req.Scopes)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create client"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "client_id":     clientID,
        "client_secret": clientSecret,
        "message":       "OAuth клиент успешно создан! Сохраните client_secret - он больше не будет показан",
    })
}

// OpenID Connect конфигурация (.well-known/openid-configuration)
func OIDCConfigurationHandler(c *gin.Context) {
    scheme := "http"
    if c.Request.TLS != nil {
        scheme = "https"
    }
    baseURL := scheme + "://" + c.Request.Host
    
    config := OIDCConfig{
        Issuer:                           baseURL,
        AuthorizationEndpoint:            baseURL + "/oauth/authorize",
        TokenEndpoint:                    baseURL + "/oauth/token",
        UserinfoEndpoint:                 baseURL + "/oauth/userinfo",
        JWKSUri:                          baseURL + "/oauth/jwks",
        ResponseTypesSupported:           []string{"code", "id_token", "id_token token"},
        SubjectTypesSupported:            []string{"public"},
        IDTokenSigningAlgValuesSupported: []string{"RS256"},
        ScopesSupported:                  []string{"openid", "profile", "email"},
        ClaimsSupported:                  []string{"sub", "iss", "exp", "iat", "auth_time", "name", "email"},
    }
    
    c.JSON(http.StatusOK, config)
}

// JWKS endpoint
func JWKSHander(c *gin.Context) {
    jwks := map[string]interface{}{
        "keys": []JWK{},
    }
    c.JSON(http.StatusOK, jwks)
}

// OAuth2 Authorization endpoint
func OAuthAuthorizeHandler(c *gin.Context) {
    responseType := c.Query("response_type")
    clientID := c.Query("client_id")
    redirectURI := c.Query("redirect_uri")
    scope := c.Query("scope")
    state := c.Query("state")
    _ = c.Query("nonce")
    
    var client OAuthClient
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT client_id, client_name, redirect_uris, confidential, active
        FROM oauth_clients
        WHERE client_id = $1 AND active = true
    `, clientID).Scan(&client.ClientID, &client.ClientName, &client.RedirectURIs, &client.Confidential, &client.Active)
    
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client", "error_description": "Client not found"})
        return
    }
    
    validRedirect := false
    for _, uri := range client.RedirectURIs {
        if uri == redirectURI {
            validRedirect = true
            break
        }
    }
    if !validRedirect {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri"})
        return
    }
    
    if responseType != "code" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_response_type"})
        return
    }
    
    userID, exists := c.Get("user_id")
    if !exists {
        sessionID := generateRandomString(32)
        c.Redirect(http.StatusFound, "/login?redirect=/oauth/authorize&session_id="+sessionID)
        return
    }
    
    authCode := generateRandomString(64)
    expiresAt := time.Now().Add(10 * time.Minute)
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO oauth_auth_codes (code, client_id, user_id, redirect_uri, scope, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, authCode, clientID, userID, redirectURI, []string{scope}, expiresAt)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
        return
    }
    
    redirectURL := fmt.Sprintf("%s?code=%s", redirectURI, authCode)
    if state != "" {
        redirectURL += "&state=" + state
    }
    
    c.Redirect(http.StatusFound, redirectURL)
}

// OAuth2 Token endpoint
func OAuthTokenHandler(c *gin.Context) {
    grantType := c.PostForm("grant_type")
    code := c.PostForm("code")
    redirectURI := c.PostForm("redirect_uri")
    clientID := c.PostForm("client_id")
    _ = c.PostForm("client_secret")
    refreshToken := c.PostForm("refresh_token")
    
    switch grantType {
    case "authorization_code":
        var storedCode string
        var userID uuid.UUID
        var storedRedirectURI string
        var scope []string
        var expiresAt time.Time
        
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT code, user_id, redirect_uri, scope, expires_at
            FROM oauth_auth_codes
            WHERE code = $1 AND expires_at > NOW()
        `, code).Scan(&storedCode, &userID, &storedRedirectURI, &scope, &expiresAt)
        
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
            return
        }
        
        if redirectURI != storedRedirectURI {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
            return
        }
        
        database.Pool.Exec(c.Request.Context(), "DELETE FROM oauth_auth_codes WHERE code = $1", code)
        
        accessToken := generateRandomString(64)
        newRefreshToken := generateRandomString(64)
        
        accessExpires := time.Now().Add(1 * time.Hour)
        refreshExpires := time.Now().Add(30 * 24 * time.Hour)
        
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO oauth_access_tokens (token, client_id, user_id, scope, expires_at)
            VALUES ($1, $2, $3, $4, $5)
        `, accessToken, clientID, userID, scope, accessExpires)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
            return
        }
        
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO oauth_refresh_tokens (token, access_token, client_id, user_id, expires_at)
            VALUES ($1, $2, $3, $4, $5)
        `, newRefreshToken, accessToken, clientID, userID, refreshExpires)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
            return
        }
        
        response := map[string]interface{}{
            "access_token":  accessToken,
            "token_type":    "Bearer",
            "expires_in":    3600,
            "refresh_token": newRefreshToken,
            "scope":         strings.Join(scope, " "),
        }
        
        c.JSON(http.StatusOK, response)
        
    case "refresh_token":
        var storedRefreshToken string
        var accessToken string
        var userID uuid.UUID
        
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT token, access_token, user_id
            FROM oauth_refresh_tokens
            WHERE token = $1 AND revoked = false AND expires_at > NOW()
        `, refreshToken).Scan(&storedRefreshToken, &accessToken, &userID)
        
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
            return
        }
        
        database.Pool.Exec(c.Request.Context(), "UPDATE oauth_access_tokens SET revoked = true WHERE token = $1", accessToken)
        
        newAccessToken := generateRandomString(64)
        newRefreshToken := generateRandomString(64)
        
        accessExpires := time.Now().Add(1 * time.Hour)
        refreshExpires := time.Now().Add(30 * 24 * time.Hour)
        
        var scope []string
        database.Pool.QueryRow(c.Request.Context(), "SELECT scope FROM oauth_access_tokens WHERE token = $1", accessToken).Scan(&scope)
        
        database.Pool.Exec(c.Request.Context(), `
            INSERT INTO oauth_access_tokens (token, client_id, user_id, scope, expires_at)
            VALUES ($1, $2, $3, $4, $5)
        `, newAccessToken, clientID, userID, scope, accessExpires)
        
        database.Pool.Exec(c.Request.Context(), `
            INSERT INTO oauth_refresh_tokens (token, access_token, client_id, user_id, expires_at)
            VALUES ($1, $2, $3, $4, $5)
        `, newRefreshToken, newAccessToken, clientID, userID, refreshExpires)
        
        database.Pool.Exec(c.Request.Context(), "UPDATE oauth_refresh_tokens SET revoked = true WHERE token = $1", refreshToken)
        
        response := map[string]interface{}{
            "access_token":  newAccessToken,
            "token_type":    "Bearer",
            "expires_in":    3600,
            "refresh_token": newRefreshToken,
        }
        
        c.JSON(http.StatusOK, response)
        
    default:
        c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
    }
}

// UserInfo endpoint
func OAuthUserInfoHandler(c *gin.Context) {
    authHeader := c.GetHeader("Authorization")
    if authHeader == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_authorization"})
        return
    }
    
    token := strings.TrimPrefix(authHeader, "Bearer ")
    
    var userID uuid.UUID
    var scope []string
    var expiresAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT user_id, scope, expires_at
        FROM oauth_access_tokens
        WHERE token = $1 AND revoked = false AND expires_at > NOW()
    `, token).Scan(&userID, &scope, &expiresAt)
    
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
        return
    }
    
    var user struct {
        Name  string
        Email string
    }
    
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT name, email FROM users WHERE id = $1
    `, userID).Scan(&user.Name, &user.Email)
    
    response := map[string]interface{}{
        "sub":   userID.String(),
        "name":  user.Name,
        "email": user.Email,
    }
    
    c.JSON(http.StatusOK, response)
}

// Вспомогательная функция для генерации случайных строк
func generateRandomString(length int) string {
    bytes := make([]byte, length)
    rand.Read(bytes)
    return base64.URLEncoding.EncodeToString(bytes)[:length]
}

// DeveloperPortalHandler - страница для разработчиков
func DeveloperPortalHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "developer-portal", gin.H{
        "title": "Developer Portal | Business Stack Identity Hub",
    })
}

// ========== РАСШИРЕННЫЕ ФУНКЦИИ IDENTITY HUB ==========

// GetIdentityHubStats - получить статистику (с разграничением по ролям)
func GetIdentityHubStats(c *gin.Context) {
    userID, _ := c.Get("user_id")
    role := c.GetString("role")
    tenantID := c.GetString("tenant_id")
    
    ctx := c.Request.Context()
    
    var totalUsers int64
    var totalClients int64
    var activeSessions int64
    var totalConsents int64
    var todayLogins int64
    var weekLogins int64
    var monthLogins int64
    
    // Если админ или разработчик - показывает общую статистику
    if role == "admin" || role == "developer" {
        // Общее количество пользователей
        database.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE tenant_id = $1", tenantID).Scan(&totalUsers)
        
        // Количество OAuth клиентов
        database.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM oauth_clients WHERE status = 'active'").Scan(&totalClients)
        
        // Активные сессии за последние 30 минут
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM user_sessions 
            WHERE last_active > NOW() - INTERVAL '30 minutes' AND revoked = false
        `).Scan(&activeSessions)
        
        // Количество согласий
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM oauth_authorizations WHERE revoked = false
        `).Scan(&totalConsents)
        
        // Логины за сегодня
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE action = 'login' AND DATE(created_at) = CURRENT_DATE
        `).Scan(&todayLogins)
        
        // Логины за неделю
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE action = 'login' AND created_at > NOW() - INTERVAL '7 days'
        `).Scan(&weekLogins)
        
        // Логины за месяц
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE action = 'login' AND created_at > NOW() - INTERVAL '30 days'
        `).Scan(&monthLogins)
    } else {
        // Обычный пользователь - видит только свою статистику
        totalUsers = 1
        
        // Количество его сессий
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM user_sessions 
            WHERE user_id = $1 AND revoked = false
        `, userID).Scan(&activeSessions)
        if activeSessions == 0 {
            activeSessions = 1
        }
        
        // Количество его согласий
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM oauth_authorizations 
            WHERE user_id = $1 AND revoked = false
        `, userID).Scan(&totalConsents)
        
        // Его логины за сегодня
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE user_id = $1 AND action = 'login' AND DATE(created_at) = CURRENT_DATE
        `, userID).Scan(&todayLogins)
        if todayLogins == 0 {
            todayLogins = 1
        }
        
        // Его логины за неделю
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE user_id = $1 AND action = 'login' AND created_at > NOW() - INTERVAL '7 days'
        `, userID).Scan(&weekLogins)
        
        // Его логины за месяц
        database.Pool.QueryRow(ctx, `
            SELECT COUNT(*) FROM activity_logs 
            WHERE user_id = $1 AND action = 'login' AND created_at > NOW() - INTERVAL '30 days'
        `, userID).Scan(&monthLogins)
        
        totalClients = 0
    }
    
    c.JSON(200, gin.H{
        "total_users":      totalUsers,
        "total_clients":    totalClients,
        "active_sessions":  activeSessions,
        "total_consents":   totalConsents,
        "today_logins":     todayLogins,
        "week_logins":      weekLogins,
        "month_logins":     monthLogins,
    })
}

// GetUserSessionsList - получает ТОЛЬКО свои сессии для обычных пользователей
func GetUserSessionsList(c *gin.Context) {
    userIDVal, exists := c.Get("user_id")
    if !exists {
        c.JSON(401, gin.H{"error": "User not authenticated"})
        return
    }
    
    userID := fmt.Sprintf("%v", userIDVal)
    role := c.GetString("role")
    ctx := c.Request.Context()
    
    var rows pgx.Rows
    var err error
    
    // Админ видит все сессии, обычный пользователь - только свои
    if role == "admin" || role == "developer" {
        rows, err = database.Pool.Query(ctx, `
            SELECT id, COALESCE(device_name, 'Unknown') as device, 
                   COALESCE(ip, '0.0.0.0') as ip, 
                   COALESCE(location, 'Unknown') as location, 
                   COALESCE(last_active, NOW()) as last_active, 
                   COALESCE(is_current, false) as is_current
            FROM user_sessions
            WHERE COALESCE(revoked, false) = false
            ORDER BY last_active DESC
            LIMIT 50
        `)
    } else {
        rows, err = database.Pool.Query(ctx, `
            SELECT id, COALESCE(device_name, 'Unknown') as device, 
                   COALESCE(ip, '0.0.0.0') as ip, 
                   COALESCE(location, 'Unknown') as location, 
                   COALESCE(last_active, NOW()) as last_active, 
                   COALESCE(is_current, false) as is_current
            FROM user_sessions
            WHERE user_id = $1 AND COALESCE(revoked, false) = false
            ORDER BY last_active DESC
        `, userID)
    }
    
    if err != nil {
        c.JSON(200, gin.H{"sessions": []gin.H{}})
        return
    }
    defer rows.Close()
    
    sessionsList := []gin.H{}
    for rows.Next() {
        var id, device, ip, location string
        var lastActive time.Time
        var isCurrent bool
        
        if err := rows.Scan(&id, &device, &ip, &location, &lastActive, &isCurrent); err != nil {
            continue
        }
        
        sessionsList = append(sessionsList, gin.H{
            "id":          id,
            "device":      device,
            "ip":          ip,
            "location":    location,
            "last_active": lastActive.Format("2006-01-02 15:04:05"),
            "is_current":  isCurrent,
        })
    }
    
    c.JSON(200, gin.H{"sessions": sessionsList})
}

// RevokeUserSession - отозвать сессию (РЕАЛЬНЫЕ ДАННЫЕ)
func RevokeUserSession(c *gin.Context) {
    userIDVal, _ := c.Get("user_id")
    userID := userIDVal.(string)
    sessionID := c.Param("id")
    
    ctx := c.Request.Context()
    
    result, err := database.Pool.Exec(ctx, `
        UPDATE user_sessions 
        SET revoked = true, is_current = false
        WHERE id = $1 AND user_id = $2 AND revoked = false
    `, sessionID, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(404, gin.H{"error": "Session not found"})
        return
    }
    
    // Логируем действие
    LogActivity(userID, "revoke_session", "security", sessionID, 
        gin.H{"session_id": sessionID}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "Сессия отозвана"})
}

// GetConnectedApps - получить список подключенных приложений (РЕАЛЬНЫЕ ДАННЫЕ)
func GetConnectedApps(c *gin.Context) {
    userIDVal, exists := c.Get("user_id")
    if !exists {
        c.JSON(401, gin.H{"error": "User not authenticated"})
        return
    }
    
    userID := userIDVal.(string)
    ctx := c.Request.Context()
    
    rows, err := database.Pool.Query(ctx, `
        SELECT oc.id, oc.name, COALESCE(oc.icon, 'bi-app') as icon, 
               COALESCE(oa.scopes, 'openid profile') as scopes, 
               oa.updated_at as last_used
        FROM oauth_authorizations oa
        JOIN oauth_clients oc ON oc.id = oa.client_id
        WHERE oa.user_id = $1 AND oa.revoked = false AND oc.status = 'active'
        ORDER BY oa.updated_at DESC
    `, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    appsList := []gin.H{}
    for rows.Next() {
        var id, name, icon, scopes string
        var lastUsed time.Time
        
        rows.Scan(&id, &name, &icon, &scopes, &lastUsed)
        
        appsList = append(appsList, gin.H{
            "id":        id,
            "name":      name,
            "icon":      icon,
            "scopes":    scopes,
            "last_used": lastUsed.Format("2006-01-02"),
            "status":    "active",
        })
    }
    
    c.JSON(200, gin.H{"apps": appsList})
}

// RevokeAppAccess - отозвать доступ приложения (РЕАЛЬНЫЕ ДАННЫЕ)
func RevokeAppAccess(c *gin.Context) {
    userIDVal, _ := c.Get("user_id")
    userID := userIDVal.(string)
    appID := c.Param("id")
    
    ctx := c.Request.Context()
    
    result, err := database.Pool.Exec(ctx, `
        UPDATE oauth_authorizations 
        SET revoked = true, revoked_at = NOW()
        WHERE user_id = $1 AND client_id = $2 AND revoked = false
    `, userID, appID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(404, gin.H{"error": "App authorization not found"})
        return
    }
    
    // Логируем действие
    LogActivity(userID, "revoke_app", "security", appID,
        gin.H{"app_id": appID}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "Доступ приложения отозван"})
}

// GetActivityLog - получить логи активности (РЕАЛЬНЫЕ ДАННЫЕ)
func GetActivityLog(c *gin.Context) {
    userIDVal, exists := c.Get("user_id")
    if !exists {
        c.JSON(401, gin.H{"error": "User not authenticated"})
        return
    }
    
    userID, ok := userIDVal.(string)
    if !ok {
        c.JSON(401, gin.H{"error": "Invalid user ID format"})
        return
    }
    
    ctx := c.Request.Context()
    
    // Проверим сначала есть ли данные
    var count int
    err := database.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM activity_logs WHERE user_id = $1", userID).Scan(&count)
    if err != nil {
        c.JSON(500, gin.H{"error": "Database error: " + err.Error()})
        return
    }
    
    // Если нет данных, вернем пустой массив
    if count == 0 {
        c.JSON(200, gin.H{"logs": []gin.H{}})
        return
    }
    
    rows, err := database.Pool.Query(ctx, `
        SELECT created_at, action, 
               COALESCE(details->>'description', details->>'method', action) as details, 
               COALESCE(ip, '0.0.0.0') as ip, 
               COALESCE(status, 'success') as status
        FROM activity_logs
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT 50
    `, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": "Query error: " + err.Error()})
        return
    }
    defer rows.Close()
    
    logsList := []gin.H{}
    for rows.Next() {
        var timestamp time.Time
        var action, details, ip, status string
        
        err := rows.Scan(&timestamp, &action, &details, &ip, &status)
        if err != nil {
            continue
        }
        
        actionName := getActionName(action)
        
        logsList = append(logsList, gin.H{
            "timestamp": timestamp.Format("2006-01-02 15:04:05"),
            "action":    actionName,
            "details":   details,
            "ip":        ip,
            "status":    status,
        })
    }
    
    c.JSON(200, gin.H{"logs": logsList})
}

// Вспомогательная функция для красивых названий действий
func getActionName(action string) string {
    names := map[string]string{
        "login":           "Вход в систему",
        "logout":          "Выход из системы",
        "oauth_authorize": "Авторизация приложения",
        "token_refresh":   "Обновление токена",
        "profile_update":  "Обновление профиля",
        "revoke_session":  "Отзыв сессии",
        "revoke_app":      "Отзыв доступа приложения",
        "api_call":        "API запрос",
        "password_change": "Смена пароля",
        "2fa_enable":      "Включение 2FA",
        "2fa_disable":     "Отключение 2FA",
    }
    
    if name, exists := names[action]; exists {
        return name
    }
    return action
}

// LogActivity - вспомогательная функция для записи логов
func LogActivity(userID, action, resource, resourceID string, details interface{}, ip, userAgent string) {
    ctx := context.Background()
    
    // Преобразуем details в JSON
    detailsJSON, _ := json.Marshal(details)
    
    _, err := database.Pool.Exec(ctx, `
        INSERT INTO activity_logs (user_id, action, resource, resource_id, details, ip, user_agent, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, 'success', NOW())
    `, userID, action, resource, resourceID, detailsJSON, ip, userAgent)
    
    if err != nil {
        // Логируем ошибку но не прерываем выполнение
        fmt.Printf("Ошибка записи лога активности: %v\n", err)
    }
}

// GetOAuthClientsList - получить список OAuth клиентов (для админа)
func GetOAuthClientsList(c *gin.Context) {
    ctx := c.Request.Context()
    
    rows, err := database.Pool.Query(ctx, `
        SELECT id, client_id, name, redirect_uris, scopes, status, created_at
        FROM oauth_clients
        ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    clients := []gin.H{}
    for rows.Next() {
        var id, clientID, name, scopes, status string
        var redirectURIs interface{}
        var createdAt time.Time
        
        rows.Scan(&id, &clientID, &name, &redirectURIs, &scopes, &status, &createdAt)
        
        clients = append(clients, gin.H{
            "id":          id,
            "client_id":   clientID,
            "name":        name,
            "redirect_uris": redirectURIs,
            "scopes":      scopes,
            "status":      status,
            "created_at":  createdAt.Format("2006-01-02 15:04:05"),
        })
    }
    
    c.JSON(200, gin.H{
        "clients": clients,
        "total":   len(clients),
    })
}

// IdentityHubLandingHandler - страница-ландинг для новых пользователей
func IdentityHubLandingHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "identity-landing.html", gin.H{
        "title": "Identity Hub | Управление идентификацией",
    })
}


// IdentityHubRouter - умный роутер для Identity Hub
func IdentityHubRouter(c *gin.Context) {
    // ========== ОТЛАДОЧНЫЙ ВЫВОД ==========
    fmt.Println("========================================")
    fmt.Println("🔍 IdentityHubRouter CALLED!")
    
    userID := c.GetString("user_id")
    role := c.GetString("role")
    email := c.GetString("user_email")
    
    fmt.Printf("🔍 role=%q, email=%q, userID=%q\n", role, email, userID)
    fmt.Println("========================================")
    
    // ========== ВЛАДЕЛЕЦ ПЛАТФОРМЫ - ПОЛНЫЙ ДОСТУП ==========
    if email == "dev@businesstack.ru" {
        fmt.Println("👑 ВЛАДЕЛЕЦ - полный доступ к identity-hub.html")
        c.HTML(http.StatusOK, "identity-hub.html", gin.H{
            "title":  "Identity Hub | Центр управления идентификацией",
            "userID": userID,
            "email":  email,
            "role":   role,
        })
        return
    }
    
    // ПРОВЕРКА: ЕСЛИ ПОЛЬЗОВАТЕЛЬ НЕ АВТОРИЗОВАН
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        fmt.Println("❌ USER NOT AUTHENTICATED - showing identity-landing.html")
        c.HTML(http.StatusOK, "identity-landing.html", gin.H{
            "title": "Identity Hub | 14 дней бесплатно",
        })
        return
    }
    
    // 1. РАЗРАБОТЧИК/АДМИН - полный доступ без триала
    if role == "developer" || role == "admin" || email == "dev@Business Stack.ru" {
        fmt.Println("✅ DEVELOPER/ADMIN - full access to identity-hub.html")
        c.HTML(http.StatusOK, "identity-hub.html", gin.H{
            "title":  "Identity Hub | Центр управления идентификацией",
            "userID": userID,
            "email":  email,
            "role":   role,
        })
        return
    }
    
    // 2. ОБЫЧНЫЙ ПОЛЬЗОВАТЕЛЬ - проверяем триал
    var trialEnd time.Time
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT trial_end FROM user_trials 
        WHERE user_id = $1::uuid AND module_name = 'identity_hub' AND trial_end > NOW()
    `, userID).Scan(&trialEnd)
    
    fmt.Printf("🔍 TRIAL CHECK: userID=%s, err=%v, trialEnd=%v\n", userID, err, trialEnd)
    
    // 3. ЕСЛИ ЕСТЬ АКТИВНЫЙ ТРИАЛ - показываем полную страницу
    if err == nil {
        fmt.Println("✅ TRIAL ACTIVE - showing identity-hub.html")
        c.HTML(http.StatusOK, "identity-hub.html", gin.H{
            "title":    "Identity Hub | Личный кабинет (Триал активен)",
            "userID":   userID,
            "email":    email,
            "role":     role,
            "trialEnd": trialEnd,
            "isTrial":  true,
        })
        return
    }
    
    // 4. НЕТ ДОСТУПА - показываем лендинг с кнопкой триала
    fmt.Println("❌ NO ACCESS - showing identity-landing.html")
    c.HTML(http.StatusOK, "identity-landing.html", gin.H{
        "title":  "Identity Hub | 14 дней бесплатно",
        "userID": userID,
        "email":  email,
        "role":   role,
    })
}

// StartModuleTrial - активация пробного периода
func StartModuleTrial(c *gin.Context) {
    userID := c.GetString("user_id")
    moduleName := c.Query("module")
    
    if moduleName == "" {
        moduleName = c.PostForm("module")
    }
    
    fmt.Printf("🔍 StartModuleTrial: userID=%s, module=%s\n", userID, moduleName)
    
    if userID == "" || moduleName == "" {
        c.JSON(400, gin.H{"error": "Missing parameters", "success": false})
        return
    }
    
    // Активируем триал на 14 дней
    trialEnd := time.Now().Add(14 * 24 * time.Hour)
    
    // ПРЯМОЙ INSERT без ON CONFLICT для теста
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO module_trials (user_id, module_name, trial_end, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
    `, userID, moduleName, trialEnd)
    
    if err != nil {
        // Если запись уже есть - обновляем
        _, err = database.Pool.Exec(c.Request.Context(), `
            UPDATE module_trials 
            SET trial_end = $3, updated_at = NOW()
            WHERE user_id = $1 AND module_name = $2
        `, userID, moduleName, trialEnd)
        
        if err != nil {
            fmt.Printf("❌ Error activating trial: %v\n", err)
            c.JSON(500, gin.H{
                "error":   "Failed to activate trial",
                "details": err.Error(),
                "success": false,
            })
            return
        }
    }
    
    // ========== ДОБАВИТЬ ЭТОТ БЛОК ==========
    // Если активируем триал разработчика (module = 'developer') - повышаем роль
    if moduleName == "identity_hub" {
        _, err = database.Pool.Exec(c.Request.Context(), `
            UPDATE users SET role = 'developer' WHERE id = $1 AND role = 'user'
        `, userID)
        if err != nil {
            fmt.Printf("⚠️ Ошибка повышения роли: %v\n", err)
        } else {
            fmt.Printf("✅ Роль пользователя %s повышена до developer\n", userID)
        }
    }
    // ========== КОНЕЦ БЛОКА ==========
    
    fmt.Printf("✅ Trial activated for user %s, module %s until %v\n", userID, moduleName, trialEnd)
    
    c.JSON(200, gin.H{
        "success":    true,
        "message":    "Пробный период на 14 дней активирован!",
        "trialEnd":   trialEnd,
        "moduleName": moduleName,
    })
}

// ExportActivityLog - экспорт логов активности в CSV
func ExportActivityLog(c *gin.Context) {
    userID := c.GetString("user_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT created_at, action, 
               COALESCE(details->>'description', details->>'method', action) as details,
               COALESCE(ip, '0.0.0.0') as ip,
               COALESCE(status, 'success') as status
        FROM activity_logs
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT 1000
    `, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    // Заголовки CSV
    csv := "Дата и время,Действие,Детали,IP адрес,Статус\n"
    
    for rows.Next() {
        var createdAt time.Time
        var action, details, ip, status string
        rows.Scan(&createdAt, &action, &details, &ip, &status)
        
        // Красивые названия действий
        actionName := map[string]string{
            "login":           "Вход в систему",
            "logout":          "Выход из системы",
            "oauth_authorize": "Авторизация приложения",
            "token_refresh":   "Обновление токена",
            "profile_update":  "Обновление профиля",
            "revoke_session":  "Отзыв сессии",
            "revoke_app":      "Отзыв доступа приложения",
            "2fa_enable":      "Включение 2FA",
            "2fa_disable":     "Отключение 2FA",
        }[action]
        if actionName == "" {
            actionName = action
        }
        
        // Экранируем кавычки и запятые в деталях
        details = strings.ReplaceAll(details, "\"", "\"\"")
        if strings.Contains(details, ",") || strings.Contains(details, "\n") {
            details = "\"" + details + "\""
        }
        
        csv += fmt.Sprintf("%s,%s,%s,%s,%s\n",
            createdAt.Format("2006-01-02 15:04:05"),
            actionName,
            details,
            ip,
            status)
    }
    
    // Отправляем CSV файл
    c.Header("Content-Type", "text/csv; charset=utf-8")
    c.Header("Content-Disposition", "attachment; filename=activity_log_"+time.Now().Format("2006-01-02")+".csv")
    c.String(200, csv)
}

// ==================== УПРАВЛЕНИЕ ПОЛЬЗОВАТЕЛЯМИ (CRUD) ====================

// GetAllUsers - получить список всех пользователей (только для админов)
func GetAllUsers(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, email, role, COALESCE(login, '') as login, 
               COALESCE(phone, '') as phone, email_verified, created_at, 
               COALESCE(blocked, false) as blocked
        FROM users 
        WHERE tenant_id = $1 AND deleted_at IS NULL
        ORDER BY created_at DESC
    `, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var users []gin.H
    for rows.Next() {
        var id, name, email, role, login, phone string
        var emailVerified, blocked bool
        var createdAt time.Time
        
        rows.Scan(&id, &name, &email, &role, &login, &phone, &emailVerified, &createdAt, &blocked)
        
        users = append(users, gin.H{
            "id":             id,
            "name":           name,
            "email":          email,
            "role":           role,
            "login":          login,
            "phone":          phone,
            "email_verified": emailVerified,
            "blocked":        blocked,
            "created_at":     createdAt,
        })
    }
    
    c.JSON(200, gin.H{"users": users, "total": len(users)})
}

// CreateUser - создать нового пользователя
func CreateUser(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    
    var req struct {
        Name     string `json:"name" binding:"required"`
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=6"`
        Role     string `json:"role"`
        Login    string `json:"login"`
        Phone    string `json:"phone"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Проверяем существование email
    var exists bool
    database.Pool.QueryRow(c.Request.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
    if exists {
        c.JSON(400, gin.H{"error": "Email already exists"})
        return
    }
    
    // Хешируем пароль
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to hash password"})
        return
    }
    
    if req.Role == "" {
        req.Role = "user"
    }
    
    var userID string
    err = database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO users (name, email, password_hash, role, login, phone, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        RETURNING id
    `, req.Name, req.Email, string(hashedPassword), req.Role, req.Login, req.Phone, tenantID).Scan(&userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // Логируем действие
    LogActivity(c.GetString("user_id"), "create_user", "users", userID, gin.H{"email": req.Email}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "user_id": userID, "message": "User created successfully"})
}

// UpdateUser - обновить данные пользователя
func UpdateUser(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var req struct {
        Name  string `json:"name"`
        Role  string `json:"role"`
        Phone string `json:"phone"`
        Login string `json:"login"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users 
        SET name = COALESCE($1, name),
            role = COALESCE($2, role),
            phone = COALESCE($3, phone),
            login = COALESCE($4, login),
            updated_at = NOW()
        WHERE id = $5 AND tenant_id = $6
    `, req.Name, req.Role, req.Phone, req.Login, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    LogActivity(c.GetString("user_id"), "update_user", "users", userID, gin.H{"updates": req}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "User updated"})
}

// DeleteUser - мягкое удаление пользователя
func DeleteUser(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2
    `, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // Также отзываем все сессии
    database.Pool.Exec(c.Request.Context(), `
        UPDATE user_sessions SET revoked = true WHERE user_id = $1
    `, userID)
    
    LogActivity(c.GetString("user_id"), "delete_user", "users", userID, gin.H{}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "User deleted"})
}

// BlockUser - заблокировать пользователя
func BlockUser(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET blocked = true, blocked_at = NOW() WHERE id = $1 AND tenant_id = $2
    `, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    // Отзываем все сессии
    database.Pool.Exec(c.Request.Context(), `
        UPDATE user_sessions SET revoked = true WHERE user_id = $1
    `, userID)
    
    LogActivity(c.GetString("user_id"), "block_user", "users", userID, gin.H{}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "User blocked"})
}

// UnblockUser - разблокировать пользователя
func UnblockUser(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET blocked = false, blocked_at = NULL WHERE id = $1 AND tenant_id = $2
    `, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    LogActivity(c.GetString("user_id"), "unblock_user", "users", userID, gin.H{}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "User unblocked"})
}

// ChangeUserRole - изменить роль пользователя
func ChangeUserRole(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var req struct {
        Role string `json:"role" binding:"required"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3
    `, req.Role, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    LogActivity(c.GetString("user_id"), "change_role", "users", userID, gin.H{"new_role": req.Role}, c.ClientIP(), c.Request.UserAgent())
    
    c.JSON(200, gin.H{"success": true, "message": "Role changed"})
}

// ==================== API ИНТЕГРАЦИИ (REST API для внешних сервисов) ====================

// VerifyToken - проверка токена (для внешних API)
func VerifyTokenHandler(c *gin.Context) {
    authHeader := c.GetHeader("Authorization")
    if authHeader == "" {
        c.JSON(401, gin.H{"valid": false, "error": "No token provided"})
        return
    }
    
    tokenString := strings.TrimPrefix(authHeader, "Bearer ")
    claims, err := utils.ValidateToken(tokenString)
    if err != nil {
        c.JSON(401, gin.H{"valid": false, "error": "Invalid token"})
        return
    }
    
    c.JSON(200, gin.H{
        "valid":   true,
        "user_id": claims.UserID,
        "email":   claims.Email,
        "role":    claims.Role,
    })
}

// GetUserInfoByToken - получить информацию о пользователе по токену
func GetUserInfoByToken(c *gin.Context) {
    userID := c.GetString("user_id")
    
    var user struct {
        ID    string `json:"id"`
        Name  string `json:"name"`
        Email string `json:"email"`
        Role  string `json:"role"`
    }
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, name, email, role FROM users WHERE id = $1 AND deleted_at IS NULL
    `, userID).Scan(&user.ID, &user.Name, &user.Email, &user.Role)
    
    if err != nil {
        c.JSON(404, gin.H{"error": "User not found"})
        return
    }
    
    c.JSON(200, user)
}

// ==================== ГРУППЫ И РОЛИ (RBAC) ====================

// GetRoles - список ролей
func GetRoles(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, description, permissions, created_at
        FROM roles
        WHERE tenant_id = $1 OR tenant_id IS NULL
        ORDER BY name
    `, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var roles []gin.H
    for rows.Next() {
        var id, name, description string
        var permissions json.RawMessage
        var createdAt time.Time
        
        rows.Scan(&id, &name, &description, &permissions, &createdAt)
        
        roles = append(roles, gin.H{
            "id":          id,
            "name":        name,
            "description": description,
            "permissions": permissions,
            "created_at":  createdAt,
        })
    }
    
    c.JSON(200, gin.H{"roles": roles})
}

// CreateRole - создать роль
func CreateRole(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    
    var req struct {
        Name        string          `json:"name" binding:"required"`
        Description string          `json:"description"`
        Permissions json.RawMessage `json:"permissions"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    if req.Permissions == nil {
        req.Permissions = json.RawMessage("[]")
    }
    
    var roleID string
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO roles (name, description, permissions, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, NOW())
        RETURNING id
    `, req.Name, req.Description, req.Permissions, tenantID).Scan(&roleID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"success": true, "role_id": roleID})
}

// AssignRole - назначить роль пользователю
func AssignRole(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")

    var req struct {
        RoleID string `json:"role_id" binding:"required"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Проверяем, что роль принадлежит тому же тенанту
    var roleTenantID *string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT tenant_id FROM roles WHERE id = $1
    `, req.RoleID).Scan(&roleTenantID)
    
    if err != nil {
        c.JSON(404, gin.H{"error": "Role not found"})
        return
    }
    
    // Проверяем доступ к роли
    if roleTenantID != nil && *roleTenantID != tenantID {
        c.JSON(403, gin.H{"error": "Access denied to this role"})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO user_roles (user_id, role_id, assigned_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (user_id, role_id) DO NOTHING
    `, userID, req.RoleID)

    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, gin.H{"success": true})
}

// GetUserRoles - получить роли пользователя
func GetUserRoles(c *gin.Context) {
    userID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT r.id, r.name, r.description
        FROM roles r
        JOIN user_roles ur ON ur.role_id = r.id
        WHERE ur.user_id = $1 AND (r.tenant_id = $2 OR r.tenant_id IS NULL)
    `, userID, tenantID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var roles []gin.H
    for rows.Next() {
        var id, name, description string
        rows.Scan(&id, &name, &description)
        roles = append(roles, gin.H{
            "id":          id,
            "name":        name,
            "description": description,
        })
    }
    
    c.JSON(200, gin.H{"roles": roles})
}

// StartDeveloperTrial - активация триала разработчика и повышение роли
func StartDeveloperTrial(c *gin.Context) {
    userID := c.GetString("user_id")
    
    fmt.Printf("🔍 StartDeveloperTrial: userID=%s\n", userID)
    
    if userID == "" {
        c.JSON(401, gin.H{"error": "Unauthorized", "success": false})
        return
    }
    
    // Проверяем, есть ли уже активный триал
    var existingTrial time.Time
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT trial_end FROM developer_trials 
        WHERE user_id = $1 AND trial_end > NOW()
    `, userID).Scan(&existingTrial)
    
    if err == nil {
        c.JSON(200, gin.H{
            "success": true,
            "message": "У вас уже активен пробный период разработчика!",
            "trialEnd": existingTrial,
        })
        return
    }
    
    // Активируем триал на 14 дней
    trialEnd := time.Now().Add(14 * 24 * time.Hour)
    
    // Сохраняем триал разработчика
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO developer_trials (user_id, trial_end, created_at, updated_at)
        VALUES ($1, $2, NOW(), NOW())
        ON CONFLICT (user_id) DO UPDATE SET trial_end = $2, updated_at = NOW()
    `, userID, trialEnd)
    
    if err != nil {
        fmt.Printf("❌ Error activating developer trial: %v\n", err)
        c.JSON(500, gin.H{
            "error":   "Failed to activate trial",
            "details": err.Error(),
            "success": false,
        })
        return
    }
    
    // ПОВЫШАЕМ РОЛЬ ДО DEVELOPER
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET role = 'developer' WHERE id = $1 AND role = 'user'
    `, userID)
    
    if err != nil {
        fmt.Printf("⚠️ Ошибка повышения роли: %v\n", err)
    } else {
        fmt.Printf("✅ Роль пользователя %s повышена до developer\n", userID)
    }
    
    fmt.Printf("✅ Developer trial activated for user %s until %v\n", userID, trialEnd)
    
    c.JSON(200, gin.H{
        "success":    true,
        "message":    "Пробный период разработчика на 14 дней активирован!",
        "trialEnd":   trialEnd,
    })
}