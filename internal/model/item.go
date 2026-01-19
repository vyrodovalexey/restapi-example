// Package model defines data structures used throughout the application.
package model

import (
	"errors"
	"time"
)

// Validation errors for Item.
var (
	ErrEmptyName        = errors.New("name cannot be empty")
	ErrNameTooLong      = errors.New("name cannot exceed 255 characters")
	ErrNegativePrice    = errors.New("price cannot be negative")
	ErrDescriptionLimit = errors.New("description cannot exceed 1000 characters")
)

// Validation constants.
const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
)

// Item represents a product or resource in the system.
type Item struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate checks if the Item has valid field values.
func (i *Item) Validate() error {
	if i.Name == "" {
		return ErrEmptyName
	}

	if len(i.Name) > MaxNameLength {
		return ErrNameTooLong
	}

	if i.Price < 0 {
		return ErrNegativePrice
	}

	if len(i.Description) > MaxDescriptionLength {
		return ErrDescriptionLimit
	}

	return nil
}

// APIResponse is a generic wrapper for API responses.
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewSuccessResponse creates a successful API response.
func NewSuccessResponse[T any](data T) APIResponse[T] {
	return APIResponse[T]{
		Success: true,
		Data:    data,
	}
}

// NewErrorResponse creates an error API response.
func NewErrorResponse[T any](errMsg string) APIResponse[T] {
	return APIResponse[T]{
		Success: false,
		Error:   errMsg,
	}
}

// ErrorResponse represents an error response structure.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// WebSocketMessage represents a message sent over WebSocket connection.
type WebSocketMessage struct {
	Type      string    `json:"type"`
	Value     int       `json:"value,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// WebSocket message types.
const (
	WSMessageTypeRandomValue = "random_value"
	WSMessageTypePing        = "ping"
	WSMessageTypePong        = "pong"
	WSMessageTypeError       = "error"
)

// NewRandomValueMessage creates a new WebSocket message with a random value.
func NewRandomValueMessage(value int) WebSocketMessage {
	return WebSocketMessage{
		Type:      WSMessageTypeRandomValue,
		Value:     value,
		Timestamp: time.Now().UTC(),
	}
}
