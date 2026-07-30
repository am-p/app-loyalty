package main

import (
	"context"
	"log"
	"os"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is not set")
	}

	connPool, err := pgxpool.NewWithConfig(context.Background(), config.Config())
	if err != nil {
		log.Fatal("Error while creating connection to the database!! ", err)
	}

	defer connPool.Close()

	err = repository.CreateTableQuery(connPool)
	if err != nil {
		log.Fatal("Could not create the table: ", err)
	}

	router := gin.Default()
	h := handler.UserHandler{Pool: connPool}
	router.POST("/auth/register", h.RegisterUser)
	router.POST("/auth/login", h.LoginUser)
	router.GET("/me", middleware.RequireAuth(), h.Me)
	router.Run()
}
