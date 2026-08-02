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

type UserResponse struct {
	ID           int64   `json:"id_usuario"`
	Email        string  `json:"email"`
	Name         string  `json:"nombre"`
	Role         string  `json:"rol"`
	Subscription *string `json:"suscripcion"`
}

type AuthResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"usuario"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleAuthRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}
