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

type CustomerHandler struct {
	Pool *pgxpool.Pool
}

func (h *CustomerHandler) RegisterCustomer(c *gin.Context) {
	var customer model.Customer

	if err := c.ShouldBindJSON(&customer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	if customer.Name == "" || customer.Email == "" || customer.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, email and password are required"})
		return
	}

	if err := service.RegisterCustomer(h.Pool, customer); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}

		log.Println("insert error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save customer"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "customer registered"})
}
