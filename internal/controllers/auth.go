package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"huasteca-backend/internal/config"
	"huasteca-backend/internal/database"
	"huasteca-backend/internal/middleware"
	"huasteca-backend/internal/models"
)

type AuthController struct {
	cfg *config.Config
}

func NewAuthController(cfg *config.Config) *AuthController {
	return &AuthController{cfg: cfg}
}

type GoogleAuthRequest struct {
	AccessToken string `json:"access_token" binding:"required"`
}

func validateGoogleAccessToken(accessToken string, clientID string) (map[string]interface{}, error) {
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("token o client ID vacío")
	}

	query := url.Values{}
	query.Set("access_token", accessToken)
	req, err := http.NewRequest(http.MethodGet, "https://oauth2.googleapis.com/tokeninfo?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token de Google inválido")
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	aud, ok := result["aud"].(string)
	if !ok || aud != clientID {
		return nil, fmt.Errorf("audiencia inválida")
	}
	// Verify the access token's audience against our configured OAuth client.
	aud, ok := result["aud"].(string)
	if !ok || aud != clientID {
		return nil, fmt.Errorf("audiencia inválida")
	}

	// Retrieve verified user identity from Google's UserInfo endpoint.
	userInfoReq, err := http.NewRequest(http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	userInfoReq.Header.Set("Authorization", "Bearer "+accessToken)
	userInfoResp, err := client.Do(userInfoReq)
	if err != nil {
		return nil, err
	}
	defer userInfoResp.Body.Close()
	if userInfoResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo de Google inválido")
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(userInfoResp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}
	email, ok := userInfo["email"].(string)
	if !ok || email == "" {
		return nil, fmt.Errorf("email ausente")
	}
	verified, ok := userInfo["email_verified"].(bool)
	if !ok || !verified {
		return nil, fmt.Errorf("email no verificado")
	}
	sub, ok := userInfo["sub"].(string)
	if !ok || sub == "" {
		return nil, fmt.Errorf("identificador Google ausente")
	}
	return userInfo, nil
}
func (ac *AuthController) GoogleAuth(c *gin.Context) {
	var req GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	payload, err := validateGoogleAccessToken(req.AccessToken, ac.cfg.GetGoogleClientID())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Google token"})
		return
	}

	email, _ := payload["email"].(string)
	name, _ := payload["name"].(string)
	pictureURL := ""
	if pic, ok := payload["picture"].(string); ok {
		pictureURL = pic
	}
	sub, _ := payload["sub"].(string)

	usersCollection := database.GetCollection("users")

	var existingUser models.User
	err = usersCollection.FindOne(c.Request.Context(), bson.M{"email": email}).Decode(&existingUser)

	if err == nil {
		log.Printf("[LOGIN] Existing user authenticated")
		token, err := generateJWT(existingUser.ID.Hex(), string(existingUser.Role), ac.cfg.GetJWTKey())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token, "user": existingUser})
		return
	}

	if err != mongo.ErrNoDocuments {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	newUser := models.User{
		ID:         primitive.NewObjectID(),
		Email:      email,
		Name:       name,
		PictureURL: pictureURL,
		Role:       models.RoleCLIENTE,
		GoogleID:   sub,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err = usersCollection.InsertOne(c.Request.Context(), newUser)
	if err != nil {
		log.Printf("[ERROR] Failed to create user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	log.Printf("[REGISTER] New user registered with default CLIENTE role")

	token, err := generateJWT(newUser.ID.Hex(), string(newUser.Role), ac.cfg.GetJWTKey())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": newUser})
}

func generateJWT(userID string, role string, jwtKey []byte) (string, error) {
	claims := middleware.Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

func (ac *AuthController) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	usersCollection := database.GetCollection("users")

	var user models.User
	err := usersCollection.FindOne(c.Request.Context(), bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (ac *AuthController) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req struct {
		Name       *string `json:"name"`
		PictureURL *string `json:"picture_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	updates := bson.M{"updated_at": time.Now()}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "El nombre no puede estar vacío"})
			return
		}
		updates["name"] = name
	}
	if req.PictureURL != nil {
		updates["picture_url"] = *req.PictureURL
	}

	usersCollection := database.GetCollection("users")
	var updatedUser models.User
	err := usersCollection.FindOneAndUpdate(
		c.Request.Context(),
		bson.M{"_id": userID},
		bson.M{"$set": updates},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedUser)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": updatedUser})
}
func (ac *AuthController) GetUsers(c *gin.Context) {
	usersCollection := database.GetCollection("users")

	cursor, err := usersCollection.Find(c.Request.Context(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer cursor.Close(c.Request.Context())

	var users []models.User
	if err := cursor.All(c.Request.Context(), &users); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (ac *AuthController) UploadProfileImage(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	filename, err := saveValidatedImage(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	relativePath := "/uploads/" + filename

	usersCollection := database.GetCollection("users")
	var previousUser models.User
	if err := usersCollection.FindOne(c.Request.Context(), bson.M{"_id": userID}).Decode(&previousUser); err != nil {
		_ = os.Remove(filepath.Join("./public/uploads", filename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user profile"})
		return
	}

	update := bson.M{"$set": bson.M{"picture_url": relativePath, "updated_at": time.Now()}}
	result, err := usersCollection.UpdateOne(c.Request.Context(), bson.M{"_id": userID}, update)
	if err != nil {
		_ = os.Remove(filepath.Join("./public/uploads", filename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile picture"})
		return
	}
	if result.MatchedCount == 0 {
		_ = os.Remove(filepath.Join("./public/uploads", filename))
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Remove the old local profile image only after the database update succeeds.
	oldURL := previousUser.PictureURL
	if strings.HasPrefix(oldURL, "/uploads/") {
		oldFilename := filepath.Base(oldURL)
		if oldFilename != "." && oldFilename != "/" && oldFilename != filename {
			_ = os.Remove(filepath.Join("./public/uploads", oldFilename))
		}
	}
	c.JSON(http.StatusOK, gin.H{"url": relativePath, "message": "Profile picture updated"})
}
