package main

import (
	"context"
	"log"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
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
	h := handler.CustomerHandler{Pool: connPool}
	router.POST("/customers", h.RegisterCustomer)
	router.Run()
}
