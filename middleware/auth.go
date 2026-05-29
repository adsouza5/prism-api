package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/adsouza5/prism-api/config"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	ClientID string `json:"client_id"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func IssueToken(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Key")
	for _, client := range config.Clients {
		if client.APIKey == apiKey {
			claims := &Claims{
				ClientID: client.ID,
				Role:     "client",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now()),
					Subject:   client.ID,
				},
			}
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			signed, err := token.SignedString(config.JWTSecret)
			if err != nil {
				http.Error(w, "token error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": signed, "client_id": client.ID})
			return
		}
	}
	http.Error(w, "invalid API key", http.StatusUnauthorized)
}

func ValidateJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return config.JWTSecret, nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-Client-ID", claims.ClientID)
		next.ServeHTTP(w, r)
	})
}
