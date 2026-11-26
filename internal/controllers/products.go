package controllers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"huasteca-backend/internal/database"
	"huasteca-backend/internal/models"
)


// parsePagination validates query parameters and applies a maximum page size.
func parsePagination(c *gin.Context) (int, int, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1_000_000 {
		return 0, 0, fmt.Errorf("page debe ser un entero mayor a cero")
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 {
		return 0, 0, fmt.Errorf("limit debe ser un entero mayor a cero")
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit, nil
}

func paginationResponse(page, limit int, total int64) gin.H {
	totalPages := (total + int64(limit) - 1) / int64(limit)
	return gin.H{"page": page, "limit": limit, "total": total, "totalPages": totalPages}
}

type ProductController struct{}

func NewProductController() *ProductController {
	return &ProductController{}
}

func (pc *ProductController) GetProducts(c *gin.Context) {
	productsCollection := database.GetCollection("products")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	page, limit, err := parsePagination(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter := bson.M{}
	total, err := productsCollection.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}
	cursor, err := productsCollection.Find(ctx, filter, options.Find().
		SetSkip(int64((page-1)*limit)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(ctx)

	products := make([]models.Product, 0, limit)
	if err := cursor.All(ctx, &products); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"pagination": paginationResponse(page, limit, total),
	})
}

func (pc *ProductController) GetProduct(c *gin.Context) {
	idParam := c.Param("id")
	productID, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	productsCollection := database.GetCollection("products")

	var product models.Product
	err = productsCollection.FindOne(c.Request.Context(), bson.M{"_id": productID}).Decode(&product)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}


	c.JSON(http.StatusOK, gin.H{"product": product})
}

func (pc *ProductController) CreateProduct(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	type ProductInput struct {
		Nombre      string  `json:"nombre" binding:"required"`
		Descripcion string  `json:"descripcion"`
		Precio      float64 `json:"precio" binding:"required,gt=0"`
		Stock       int     `json:"stock" binding:"gte=0"`
		ImagenURL   string  `json:"imagen_url"`
	}

	var input ProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	productsCollection := database.GetCollection("products")

	newProduct := models.Product{
		ID:          primitive.NewObjectID(),
		Nombre:      input.Nombre,
		Descripcion: input.Descripcion,
		Precio:      input.Precio,
		Stock:       input.Stock,
		ImagenURL:   input.ImagenURL,
		VendedorID:  userID.(primitive.ObjectID),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := productsCollection.InsertOne(c.Request.Context(), newProduct)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
		return
	}

	newProduct.ID = result.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusCreated, gin.H{"product": newProduct})
}

func (pc *ProductController) UpdateProduct(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	idParam := c.Param("id")
	productID, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	type ProductInput struct {
	    Nombre      *string  `json:"nombre"`
	    Descripcion *string  `json:"descripcion"`
	    Precio      *float64 `json:"precio"`
	    Stock       *int     `json:"stock"`
	    ImagenURL   *string  `json:"imagen_url"`
	}

	var input ProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	productsCollection := database.GetCollection("products")

	var existingProduct models.Product
	err = productsCollection.FindOne(c.Request.Context(), bson.M{"_id": productID}).Decode(&existingProduct)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	// Check if user is admin or the vendor
	roleVal, _ := c.Get("role")
	userRole, _ := roleVal.(models.Role)
	if userRole != models.RoleADMIN && existingProduct.VendedorID != userID.(primitive.ObjectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to update this product"})
		return
	}

	updateFields := bson.M{
	    "updated_at": time.Now(),
	}
	
	if input.Nombre != nil {
	    nombre := strings.TrimSpace(*input.Nombre)
	
	    if nombre == "" {
	        c.JSON(http.StatusBadRequest, gin.H{
	            "error": "El nombre no puede estar vacío",
	        })
	        return
	    }
	
	    updateFields["nombre"] = nombre
	}
	
	if input.Descripcion != nil {
	    updateFields["descripcion"] = *input.Descripcion
	}
	
	if input.Precio != nil {
	    if *input.Precio <= 0 {
	        c.JSON(http.StatusBadRequest, gin.H{
	            "error": "El precio debe ser mayor a cero",
	        })
	        return
	    }
	
	    updateFields["precio"] = *input.Precio
	}
	
	if input.Stock != nil {
	    if *input.Stock < 0 {
	        c.JSON(http.StatusBadRequest, gin.H{
	            "error": "El stock no puede ser negativo",
	        })
	        return
	    }
	
	    updateFields["stock"] = *input.Stock
	}
	
	if input.ImagenURL != nil {
	    updateFields["imagen_url"] = *input.ImagenURL
	}
	
	update := bson.M{
	    "$set": updateFields,
	}

	result, err := productsCollection.UpdateOne(c.Request.Context(), bson.M{"_id": productID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}
	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated"})
}

func (pc *ProductController) DeleteProduct(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	idParam := c.Param("id")
	productID, err := primitive.ObjectIDFromHex(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	productsCollection := database.GetCollection("products")

	var existingProduct models.Product
	err = productsCollection.FindOne(c.Request.Context(), bson.M{"_id": productID}).Decode(&existingProduct)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	// Check if user is admin or the vendor
	roleVal, _ := c.Get("role")
	userRole, _ := roleVal.(models.Role)
	if userRole != models.RoleADMIN && existingProduct.VendedorID != userID.(primitive.ObjectID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to delete this product"})
		return
	}

	_, err = productsCollection.DeleteOne(c.Request.Context(), bson.M{"_id": productID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted"})
}

func (pc *ProductController) UploadImage(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{"url": relativePath})
}
