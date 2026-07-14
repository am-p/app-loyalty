package model

import "time"

type RegisterRequest struct {
	Name     string `json:"nombre" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type User struct {
	ID           int64     `json:"id_usuario"`
	Email        string    `json:"email"`
	PasswordHash *string   `json:"-"`
	GoogleID     *string   `json:"google_id"`
	Name         string    `json:"nombre"`
	Role         string    `json:"rol"`
	ShopID       *int64    `json:"id_tienda"`
	ClientID     *int64    `json:"id_cliente"`
	CreatedAt    time.Time `json:"-"`
}
