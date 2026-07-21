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

type UserHandler struct {
	Pool *pgxpool.Pool
}

func (h *UserHandler) RegisterUser(c *gin.Context) {
	var req model.RegisterRequest

	if  err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: check name, email and password"})
		return
	}

	if _, err := service.RegisterUser(h.Pool, req); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}

		log.Println("insert error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save shop"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "user registered"})
}
