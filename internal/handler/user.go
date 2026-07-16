package handler

import (
	"errors"
	"log"
	"net/http"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ShopHandler struct {
	Pool *pgxpool.Pool
}

func (h *ShopHandler) RegisterShop(c *gin.Context) {
	var shop model.Shop

	if err := c.ShouldBindJSON(&shop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: check name, email and password"})
		return
	}

	if err := service.RegisterShop(h.Pool, shop); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}

		log.Println("insert error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save shop"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "shop registered"})
}
