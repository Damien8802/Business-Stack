package auth

import (
"errors"
"time"
"subscription-system/config"

"github.com/golang-jwt/jwt/v4"
)

type Claims struct {
UserID string `json:"user_id"`
Email  string `json:"email"`
Role   string `json:"role"`
Type   string `json:"type"` // "access" или "refresh"
jwt.RegisteredClaims
}

func GenerateTokenPair(cfg *config.Config, userID, email, role string) (accessToken, refreshToken string, err error) {
// Access token
accessClaims := Claims{
UserID: userID,
Email:  email,
Role:   role,
Type:   "access",
RegisteredClaims: jwt.RegisteredClaims{
ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.JWTAccessExpiry)),
IssuedAt:  jwt.NewNumericDate(time.Now()),
NotBefore: jwt.NewNumericDate(time.Now()),
Issuer:    "Business Stack",
Subject:   userID,
},
}
access := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
accessToken, err = access.SignedString([]byte(cfg.JWTSecret))
if err != nil {
return "", "", err
}

// Refresh token
refreshClaims := Claims{
UserID: userID,
Email:  email,
Role:   role,
Type:   "refresh",
RegisteredClaims: jwt.RegisteredClaims{
ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.JWTRefreshExpiry)),
IssuedAt:  jwt.NewNumericDate(time.Now()),
NotBefore: jwt.NewNumericDate(time.Now()),
Issuer:    "Business Stack",
Subject:   userID,
},
}
refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
refreshToken, err = refresh.SignedString([]byte(cfg.JWTRefreshSecret))
if err != nil {
return "", "", err
}

return accessToken, refreshToken, nil
}

func ValidateAccessToken(cfg *config.Config, tokenString string) (*Claims, error) {
token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
return []byte(cfg.JWTSecret), nil
})
if err != nil {
return nil, err
}

if claims, ok := token.Claims.(*Claims); ok && token.Valid {
if claims.Type != "access" {
return nil, errors.New("token is not an access token")
}
return claims, nil
}
return nil, errors.New("invalid access token")
}

func ValidateRefreshToken(cfg *config.Config, tokenString string) (*Claims, error) {
token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
return []byte(cfg.JWTRefreshSecret), nil
})
if err != nil {
return nil, err
}

if claims, ok := token.Claims.(*Claims); ok && token.Valid {
if claims.Type != "refresh" {
return nil, errors.New("token is not a refresh token")
}
return claims, nil
}
return nil, errors.New("invalid refresh token")
}

func RefreshTokens(cfg *config.Config, refreshTokenString string) (newAccessToken, newRefreshToken string, err error) {
claims, err := ValidateRefreshToken(cfg, refreshTokenString)
if err != nil {
return "", "", err
}
return GenerateTokenPair(cfg, claims.UserID, claims.Email, claims.Role)
}
