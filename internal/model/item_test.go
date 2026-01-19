// Package model defines data structures used throughout the application.
package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		item    Item
		wantErr error
	}{
		{
			name: "valid item",
			item: Item{
				ID:          "123",
				Name:        "Test Item",
				Description: "A test item",
				Price:       9.99,
			},
			wantErr: nil,
		},
		{
			name: "valid item - zero price",
			item: Item{
				ID:    "123",
				Name:  "Free Item",
				Price: 0,
			},
			wantErr: nil,
		},
		{
			name: "valid item - empty description",
			item: Item{
				ID:    "123",
				Name:  "Test Item",
				Price: 10.00,
			},
			wantErr: nil,
		},
		{
			name: "valid item - max name length",
			item: Item{
				ID:    "123",
				Name:  strings.Repeat("a", MaxNameLength),
				Price: 10.00,
			},
			wantErr: nil,
		},
		{
			name: "valid item - max description length",
			item: Item{
				ID:          "123",
				Name:        "Test Item",
				Description: strings.Repeat("a", MaxDescriptionLength),
				Price:       10.00,
			},
			wantErr: nil,
		},
		{
			name: "invalid - empty name",
			item: Item{
				ID:    "123",
				Name:  "",
				Price: 10.00,
			},
			wantErr: ErrEmptyName,
		},
		{
			name: "invalid - name too long",
			item: Item{
				ID:    "123",
				Name:  strings.Repeat("a", MaxNameLength+1),
				Price: 10.00,
			},
			wantErr: ErrNameTooLong,
		},
		{
			name: "invalid - negative price",
			item: Item{
				ID:    "123",
				Name:  "Test Item",
				Price: -1.00,
			},
			wantErr: ErrNegativePrice,
		},
		{
			name: "invalid - description too long",
			item: Item{
				ID:          "123",
				Name:        "Test Item",
				Description: strings.Repeat("a", MaxDescriptionLength+1),
				Price:       10.00,
			},
			wantErr: ErrDescriptionLimit,
		},
		{
			name: "invalid - very negative price",
			item: Item{
				ID:    "123",
				Name:  "Test Item",
				Price: -999999.99,
			},
			wantErr: ErrNegativePrice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.item.Validate()

			// Assert
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestItem_JSONMarshal(t *testing.T) {
	// Arrange
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	item := Item{
		ID:          "test-id-123",
		Name:        "Test Item",
		Description: "A test description",
		Price:       19.99,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Act
	data, err := json.Marshal(item)

	// Assert
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() unexpected error: %v", err)
	}

	if result["id"] != "test-id-123" {
		t.Errorf("id = %v, want test-id-123", result["id"])
	}
	if result["name"] != "Test Item" {
		t.Errorf("name = %v, want Test Item", result["name"])
	}
	if result["description"] != "A test description" {
		t.Errorf("description = %v, want A test description", result["description"])
	}
	if result["price"] != 19.99 {
		t.Errorf("price = %v, want 19.99", result["price"])
	}
}

func TestItem_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    Item
		wantErr bool
	}{
		{
			name: "valid JSON",
			json: `{"id":"123","name":"Test Item","description":"Test","price":9.99}`,
			want: Item{
				ID:          "123",
				Name:        "Test Item",
				Description: "Test",
				Price:       9.99,
			},
			wantErr: false,
		},
		{
			name: "JSON without optional fields",
			json: `{"id":"123","name":"Test Item","price":9.99}`,
			want: Item{
				ID:    "123",
				Name:  "Test Item",
				Price: 9.99,
			},
			wantErr: false,
		},
		{
			name: "JSON with zero price",
			json: `{"id":"123","name":"Free Item","price":0}`,
			want: Item{
				ID:    "123",
				Name:  "Free Item",
				Price: 0,
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			json:    `{"id":"123","name":}`,
			want:    Item{},
			wantErr: true,
		},
		{
			name:    "empty JSON",
			json:    `{}`,
			want:    Item{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var item Item

			// Act
			err := json.Unmarshal([]byte(tt.json), &item)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Errorf("json.Unmarshal() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("json.Unmarshal() unexpected error: %v", err)
			}

			if item.ID != tt.want.ID {
				t.Errorf("ID = %s, want %s", item.ID, tt.want.ID)
			}
			if item.Name != tt.want.Name {
				t.Errorf("Name = %s, want %s", item.Name, tt.want.Name)
			}
			if item.Description != tt.want.Description {
				t.Errorf("Description = %s, want %s", item.Description, tt.want.Description)
			}
			if item.Price != tt.want.Price {
				t.Errorf("Price = %f, want %f", item.Price, tt.want.Price)
			}
		})
	}
}

func TestItem_JSONOmitEmpty(t *testing.T) {
	// Arrange - Item with empty description
	item := Item{
		ID:    "123",
		Name:  "Test Item",
		Price: 9.99,
	}

	// Act
	data, err := json.Marshal(item)

	// Assert
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"description"`) {
		t.Errorf("JSON should omit empty description, got: %s", jsonStr)
	}
}

func TestAPIResponse_Success(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "string data",
			data: "test",
		},
		{
			name: "item data",
			data: Item{ID: "123", Name: "Test"},
		},
		{
			name: "slice data",
			data: []Item{{ID: "1"}, {ID: "2"}},
		},
		{
			name: "nil data",
			data: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			resp := NewSuccessResponse(tt.data)

			// Assert
			if !resp.Success {
				t.Errorf("Success = false, want true")
			}
			if resp.Error != "" {
				t.Errorf("Error = %s, want empty string", resp.Error)
			}
		})
	}
}

func TestAPIResponse_Error(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
	}{
		{
			name:   "simple error",
			errMsg: "something went wrong",
		},
		{
			name:   "empty error",
			errMsg: "",
		},
		{
			name:   "detailed error",
			errMsg: "validation failed: name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			resp := NewErrorResponse[any](tt.errMsg)

			// Assert
			if resp.Success {
				t.Errorf("Success = true, want false")
			}
			if resp.Error != tt.errMsg {
				t.Errorf("Error = %s, want %s", resp.Error, tt.errMsg)
			}
		})
	}
}

func TestAPIResponse_JSONMarshal(t *testing.T) {
	// Arrange
	resp := NewSuccessResponse(Item{ID: "123", Name: "Test"})

	// Act
	data, err := json.Marshal(resp)

	// Assert
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() unexpected error: %v", err)
	}

	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
	if result["data"] == nil {
		t.Errorf("data should not be nil")
	}
}

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		response ErrorResponse
	}{
		{
			name: "basic error response",
			response: ErrorResponse{
				Code:    400,
				Message: "Bad Request",
			},
		},
		{
			name: "error response with details",
			response: ErrorResponse{
				Code:    422,
				Message: "Validation Error",
				Details: "name field is required",
			},
		},
		{
			name: "internal server error",
			response: ErrorResponse{
				Code:    500,
				Message: "Internal Server Error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			data, err := json.Marshal(tt.response)

			// Assert
			if err != nil {
				t.Fatalf("json.Marshal() unexpected error: %v", err)
			}

			var result ErrorResponse
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("json.Unmarshal() unexpected error: %v", err)
			}

			if result.Code != tt.response.Code {
				t.Errorf("Code = %d, want %d", result.Code, tt.response.Code)
			}
			if result.Message != tt.response.Message {
				t.Errorf("Message = %s, want %s", result.Message, tt.response.Message)
			}
			if result.Details != tt.response.Details {
				t.Errorf("Details = %s, want %s", result.Details, tt.response.Details)
			}
		})
	}
}

func TestErrorResponse_JSONOmitEmpty(t *testing.T) {
	// Arrange - ErrorResponse without details
	resp := ErrorResponse{
		Code:    400,
		Message: "Bad Request",
	}

	// Act
	data, err := json.Marshal(resp)

	// Assert
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"details"`) {
		t.Errorf("JSON should omit empty details, got: %s", jsonStr)
	}
}

func TestWebSocketMessage(t *testing.T) {
	tests := []struct {
		name    string
		message WebSocketMessage
	}{
		{
			name: "random value message",
			message: WebSocketMessage{
				Type:      WSMessageTypeRandomValue,
				Value:     42,
				Timestamp: time.Now().UTC(),
			},
		},
		{
			name: "ping message",
			message: WebSocketMessage{
				Type:      WSMessageTypePing,
				Timestamp: time.Now().UTC(),
			},
		},
		{
			name: "pong message",
			message: WebSocketMessage{
				Type:      WSMessageTypePong,
				Timestamp: time.Now().UTC(),
			},
		},
		{
			name: "error message",
			message: WebSocketMessage{
				Type:      WSMessageTypeError,
				Timestamp: time.Now().UTC(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			data, err := json.Marshal(tt.message)

			// Assert
			if err != nil {
				t.Fatalf("json.Marshal() unexpected error: %v", err)
			}

			var result WebSocketMessage
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("json.Unmarshal() unexpected error: %v", err)
			}

			if result.Type != tt.message.Type {
				t.Errorf("Type = %s, want %s", result.Type, tt.message.Type)
			}
			if result.Value != tt.message.Value {
				t.Errorf("Value = %d, want %d", result.Value, tt.message.Value)
			}
		})
	}
}

func TestNewRandomValueMessage(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{
			name:  "positive value",
			value: 42,
		},
		{
			name:  "zero value",
			value: 0,
		},
		{
			name:  "large value",
			value: 999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			before := time.Now().UTC()

			// Act
			msg := NewRandomValueMessage(tt.value)

			// Assert
			after := time.Now().UTC()

			if msg.Type != WSMessageTypeRandomValue {
				t.Errorf("Type = %s, want %s", msg.Type, WSMessageTypeRandomValue)
			}
			if msg.Value != tt.value {
				t.Errorf("Value = %d, want %d", msg.Value, tt.value)
			}
			if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
				t.Errorf("Timestamp = %v, should be between %v and %v", msg.Timestamp, before, after)
			}
		})
	}
}

func TestWebSocketMessageConstants(t *testing.T) {
	// Assert that constants have expected values
	if WSMessageTypeRandomValue != "random_value" {
		t.Errorf("WSMessageTypeRandomValue = %s, want random_value", WSMessageTypeRandomValue)
	}
	if WSMessageTypePing != "ping" {
		t.Errorf("WSMessageTypePing = %s, want ping", WSMessageTypePing)
	}
	if WSMessageTypePong != "pong" {
		t.Errorf("WSMessageTypePong = %s, want pong", WSMessageTypePong)
	}
	if WSMessageTypeError != "error" {
		t.Errorf("WSMessageTypeError = %s, want error", WSMessageTypeError)
	}
}

func TestValidationConstants(t *testing.T) {
	// Assert that constants have expected values
	if MaxNameLength != 255 {
		t.Errorf("MaxNameLength = %d, want 255", MaxNameLength)
	}
	if MaxDescriptionLength != 1000 {
		t.Errorf("MaxDescriptionLength = %d, want 1000", MaxDescriptionLength)
	}
}
