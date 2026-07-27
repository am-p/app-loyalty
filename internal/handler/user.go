package handler

import (
	"errors"
	"log"
	"net/http"

	"clientesFrecuentes/internal/auth"
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

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Datos inválidos: revisa nombre, email y contraseña"})
		return
	}

	id, err := service.RegisterUser(h.Pool, req)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"message": "El email ya está registrado"})
			return
		}

		log.Println("insert error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "No pudimos completar el registro, intenta más tarde"})
		return
	}

	token, err := auth.GenerateToken(id, "CLIENTE_FINAL")
	if err != nil {
		log.Println("token error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "No pudimos completar el registro, intenta más tarde"})
		return
	}

	c.JSON(http.StatusCreated, model.AuthResponse{
		Token: token,
		User: model.UserResponse{
			ID:    id,
			Email: req.Email,
			Name:  req.Name,
			Role:  "CLIENTE_FINAL",
		},
	})
}

func (h *UserHandler) LoginUser(c *gin.Context) {
	var req model.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Datos inválidos: email y contraseña"})
		return
	}

	user, err := service.LoginUser(h.Pool, req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "El correo o la contraseña son incorrectos"})
			return
		}

		log.Println("login error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "No pudimos iniciar sesión, intenta más tarde"})
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Role)
	if err != nil {
		log.Println("token error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "No pudimos iniciar sesión, intenta más tarde"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{
		Token: token,
		User: model.UserResponse{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name,
			Role:  user.Role,
		},
	})
}
