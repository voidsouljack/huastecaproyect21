package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"huasteca-backend/internal/database"
	"huasteca-backend/internal/models"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("JWT signing method not allowed")
			}
			return []byte(jwtSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuedAt())

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user ID"})
			c.Abort()
			return
		}

		if claims.ExpiresAt == nil || claims.IssuedAt == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token expiration and issued-at claims required"})
			c.Abort()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		var currentUser models.User
		err = database.GetCollection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&currentUser)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found or not authenticated"})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
			}
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("role", currentUser.Role)
		c.Next()
	}
}

func RBACMiddleware(allowedRoles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Role not found in context"})
			c.Abort()
			return
		}

		userRole := roleVal.(models.Role)
		for _, role := range allowedRoles {
			if userRole == role {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		c.Abort()
	}
}