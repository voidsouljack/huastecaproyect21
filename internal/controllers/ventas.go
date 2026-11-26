package controllers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"huasteca-backend/internal/database"
	"huasteca-backend/internal/models"
)

type VentasController struct{}

func NewVentasController() *VentasController {
	return &VentasController{}
}

func (vc *VentasController) GetVentas(c *gin.Context) {
	userID, _ := c.Get("user_id")
	roleVal, _ := c.Get("role")
	role := roleVal.(models.Role)

	ventasCollection := database.GetCollection("ventas")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	switch role {
	case models.RoleADMIN:
		// Admin can view all sales.
	case models.RoleVENDEDOR:
		filter["vendedor_id"] = userID
	default:
		filter["comprador_id"] = userID
	}

	page, limit, err := parsePagination(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	total, err := ventasCollection.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error counting sales"})
		return
	}
	cursor, err := ventasCollection.Find(ctx, filter, options.Find().
		SetSkip(int64((page-1)*limit)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "fecha", Value: -1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching sales"})
		return
	}
	defer cursor.Close(ctx)

	ventas := make([]models.Venta, 0, limit)
	if err := cursor.All(ctx, &ventas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding sales"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ventas": ventas,
		"pagination": paginationResponse(page, limit, total),
	})
}

func (vc *VentasController) CreateVenta(c *gin.Context) {
	userIDVal, _ := c.Get("user_id")
	userID := userIDVal.(primitive.ObjectID)

	type VentaInput struct {
		ProductoID string `json:"producto_id" binding:"required"`
		Cantidad   int    `json:"cantidad" binding:"required,gt=0"`
	}

	var input VentaInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	productoID, err := primitive.ObjectIDFromHex(input.ProductoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID"})
		return
	}

	productsCollection := database.GetCollection("products")
	ventasCollection := database.GetCollection("ventas")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	session, err := database.Client.StartSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo iniciar la compra"})
		return
	}
	defer session.EndSession(ctx)

	var ventaCreada models.Venta
	var productoNoExiste bool
	var stockInsuficiente bool

	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		productoNoExiste = false
		stockInsuficiente = false

		var product models.Product
		err := productsCollection.FindOne(sc, bson.M{"_id": productoID}).Decode(&product)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				productoNoExiste = true
			}
			return nil, err
		}

		stockResult, err := productsCollection.UpdateOne(
			sc,
			bson.M{"_id": productoID, "stock": bson.M{"$gte": input.Cantidad}},
			bson.M{"$inc": bson.M{"stock": -input.Cantidad}, "$set": bson.M{"updated_at": time.Now()}},
		)
		if err != nil {
			return nil, err
		}
		if stockResult.MatchedCount == 0 {
			stockInsuficiente = true
			return nil, fmt.Errorf("stock insuficiente")
		}

		ventaCreada = models.Venta{
			ID:             primitive.NewObjectID(),
			ProductoID:     productoID,
			ProductoNombre: product.Nombre,
			Cantidad:       input.Cantidad,
			PrecioUnitario: product.Precio,
			Total:          product.Precio * float64(input.Cantidad),
			CompradorID:    userID,
			VendedorID:     product.VendedorID,
			Fecha:          time.Now(),
		}

		_, err = ventasCollection.InsertOne(sc, ventaCreada)
		if err != nil {
			return nil, err
		}
		return ventaCreada, nil
	})

	if err != nil {
		if productoNoExiste {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		if stockInsuficiente {
			c.JSON(http.StatusConflict, gin.H{"error": "Stock insuficiente"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo completar la venta"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Purchase completed", "venta": ventaCreada})
}
