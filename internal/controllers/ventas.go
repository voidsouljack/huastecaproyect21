package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

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

	var cursor *mongo.Cursor
	var err error

	switch role {
	case models.RoleADMIN:
		cursor, err = ventasCollection.Find(ctx, bson.M{})
	case models.RoleVENDEDOR:
		cursor, err = ventasCollection.Find(ctx, bson.M{"vendedor_id": userID})
	default:
		cursor, err = ventasCollection.Find(ctx, bson.M{"comprador_id": userID})
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching sales"})
		return
	}
	defer cursor.Close(ctx)

	var ventas []models.Venta
	if err := cursor.All(ctx, &ventas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding sales"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ventas": ventas})
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

ctx, cancel := context.WithTimeout(
    c.Request.Context(),
    10*time.Second,
)
defer cancel()

session, err := database.Client.StartSession()
if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{
        "error": "No se pudo iniciar la transacción",
    })
    return
}
defer session.EndSession(ctx)

var ventaCreada models.Venta

_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {

    // 1. Obtener producto dentro de la transacción.
    var product models.Product

    err := productsCollection.FindOne(
        sc,
        bson.M{"_id": productoID},
    ).Decode(&product)

    if err != nil {
        return nil, err
    }

    // 2. Descontar inventario de forma atómica.
    stockResult, err := productsCollection.UpdateOne(
        sc,
        bson.M{
            "_id": productoID,
            "stock": bson.M{
                "$gte": input.Cantidad,
            },
        },
        bson.M{
            "$inc": bson.M{
                "stock": -input.Cantidad,
            },
            "$set": bson.M{
                "updated_at": time.Now(),
            },
        },
    )

    if err != nil {
        return nil, err
    }

    if stockResult.MatchedCount == 0 {
        return nil, fmt.Errorf("stock insuficiente")
    }

    // 3. Registrar la venta.
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
    c.JSON(http.StatusConflict, gin.H{
        "error": "No se pudo completar la compra. Verifica el stock disponible.",
    })
    return
}

c.JSON(http.StatusCreated, gin.H{
    "message": "Purchase completed",
    "venta":   ventaCreada,
})
}
