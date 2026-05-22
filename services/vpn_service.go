package services

import (
    "context"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "time"
    
    "github.com/jackc/pgx/v5/pgxpool"
)

type VPNService struct {
    db *pgxpool.Pool
}

func NewVPNService(db *pgxpool.Pool) *VPNService {
    return &VPNService{db: db}
}

func (s *VPNService) GenerateKey(tenantID, clientName string, planID int) (map[string]interface{}, error) {
    clientIP := fmt.Sprintf("10.0.0.%d", time.Now().UnixNano()%254+1)
    privateKey := generatePrivateKey()
    publicKey := generatePublicKey()
    
    var planName string
    var days int
    err := s.db.QueryRow(context.Background(), `
        SELECT name, days FROM vpn_plans WHERE id = $1
    `, planID).Scan(&planName, &days)
    if err != nil {
        return nil, err
    }
    
    expiresAt := time.Now().AddDate(0, 0, days)
    
    _, err = s.db.Exec(context.Background(), `
        INSERT INTO vpn_keys (client_name, client_ip, private_key, public_key, plan_id, expires_at, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
    `, clientName, clientIP, privateKey, publicKey, planID, expiresAt, tenantID)
    
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "client_name": clientName,
        "client_ip":   clientIP,
        "private_key": privateKey,
        "public_key":  publicKey,
        "plan":        planName,
        "expires_at":  expiresAt,
    }, nil
}

func (s *VPNService) GetKeys(tenantID string) ([]map[string]interface{}, error) {
    rows, err := s.db.Query(context.Background(), `
        SELECT k.id, k.client_name, k.client_ip, k.expires_at, k.active, k.created_at,
               p.name as plan_name, p.speed, p.devices
        FROM vpn_keys k
        LEFT JOIN vpn_plans p ON k.plan_id = p.id
        WHERE k.tenant_id = $1
        ORDER BY k.created_at DESC
    `, tenantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var keys []map[string]interface{}
    for rows.Next() {
        var id int
        var clientName, clientIP, planName, speed string
        var devices int
        var expiresAt, createdAt time.Time
        var active bool
        
        rows.Scan(&id, &clientName, &clientIP, &expiresAt, &active, &createdAt, &planName, &speed, &devices)
        
        keys = append(keys, map[string]interface{}{
            "id":          id,
            "client_name": clientName,
            "client_ip":   clientIP,
            "plan":        planName,
            "speed":       speed,
            "devices":     devices,
            "expires_at":  expiresAt,
            "created_at":  createdAt,
            "active":      active,
        })
    }
    return keys, nil
}

func (s *VPNService) GetStats(tenantID string) (map[string]interface{}, error) {
    var total, active, expired int
    err := s.db.QueryRow(context.Background(), `
        SELECT 
            COUNT(*) as total,
            COUNT(*) FILTER (WHERE active = true AND expires_at > NOW()) as active,
            COUNT(*) FILTER (WHERE expires_at <= NOW()) as expired
        FROM vpn_keys WHERE tenant_id = $1
    `, tenantID).Scan(&total, &active, &expired)
    
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "total_keys":   total,
        "active_keys":  active,
        "expired_keys": expired,
    }, nil
}

func (s *VPNService) GetConfig(keyID int, tenantID string) (string, error) {
    var privateKey, clientIP string
    err := s.db.QueryRow(context.Background(), `
        SELECT private_key, client_ip FROM vpn_keys 
        WHERE id = $1 AND tenant_id = $2 AND active = true
    `, keyID, tenantID).Scan(&privateKey, &clientIP)
    if err != nil {
        return "", err
    }
    
    config := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/24
DNS = 8.8.8.8, 1.1.1.1

[Peer]
PublicKey = SERVER_PUBLIC_KEY
Endpoint = vpn.Business Stack.ru:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25`, privateKey, clientIP)
    
    return config, nil
}

func (s *VPNService) RevokeKey(keyID int, tenantID string) error {
    _, err := s.db.Exec(context.Background(), `
        UPDATE vpn_keys SET active = false WHERE id = $1 AND tenant_id = $2
    `, keyID, tenantID)
    return err
}

func generatePrivateKey() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.StdEncoding.EncodeToString(b)
}

func generatePublicKey() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.StdEncoding.EncodeToString(b)
}