// graphql.go implements the GraphQL HTTP handler for querying and mutating items.
// It exposes a /graphql endpoint supporting both POST queries/mutations and
// a GET-accessible GraphiQL playground.

package handler

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/graphql-go/graphql"
	gqlhandler "github.com/graphql-go/handler"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// GraphQLHandler handles GraphQL API requests for items.
type GraphQLHandler struct {
	store   store.Store
	logger  *zap.Logger
	schema  graphql.Schema
	handler *gqlhandler.Handler
}

// NewGraphQLHandler creates a new GraphQLHandler instance.
// It panics if the GraphQL schema cannot be built, which indicates a programming error.
func NewGraphQLHandler(s store.Store, logger *zap.Logger) *GraphQLHandler {
	h := &GraphQLHandler{
		store:  s,
		logger: logger,
	}

	schema, err := h.buildSchema()
	if err != nil {
		// Schema build failure is a programming error — fail fast.
		panic(fmt.Sprintf("failed to create GraphQL schema: %v", err))
	}

	h.schema = schema
	h.handler = gqlhandler.New(&gqlhandler.Config{
		Schema:   &h.schema,
		Pretty:   true,
		GraphiQL: true,
	})

	return h
}

// RegisterRoutes registers the GraphQL routes with the router.
func (h *GraphQLHandler) RegisterRoutes(router *mux.Router) {
	router.Handle("/graphql", h.handler).Methods(http.MethodPost, http.MethodGet)
}

// buildSchema constructs the GraphQL schema with all types, queries, and mutations.
func (h *GraphQLHandler) buildSchema() (graphql.Schema, error) {
	itemType := h.buildItemType()
	createItemInput := h.buildCreateItemInput()
	updateItemInput := h.buildUpdateItemInput()

	queryType := h.buildQueryType(itemType)
	mutationType := h.buildMutationType(itemType, createItemInput, updateItemInput)

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
	if err != nil {
		return graphql.Schema{}, fmt.Errorf("building GraphQL schema: %w", err)
	}

	return schema, nil
}

// buildItemType defines the GraphQL Item object type.
func (h *GraphQLHandler) buildItemType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "Item",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
			},
			"name": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
			},
			"description": &graphql.Field{
				Type: graphql.String,
			},
			"price": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Float),
			},
			"createdAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					if item, ok := p.Source.(*model.Item); ok {
						return item.CreatedAt.Format(time.RFC3339), nil
					}
					return nil, nil
				},
			},
			"updatedAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					if item, ok := p.Source.(*model.Item); ok {
						return item.UpdatedAt.Format(time.RFC3339), nil
					}
					return nil, nil
				},
			},
		},
	})
}

// buildCreateItemInput defines the GraphQL input type for creating items.
func (h *GraphQLHandler) buildCreateItemInput() *graphql.InputObject {
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateItemInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name": &graphql.InputObjectFieldConfig{
				Type: graphql.NewNonNull(graphql.String),
			},
			"description": &graphql.InputObjectFieldConfig{
				Type: graphql.String,
			},
			"price": &graphql.InputObjectFieldConfig{
				Type: graphql.NewNonNull(graphql.Float),
			},
		},
	})
}

// buildUpdateItemInput defines the GraphQL input type for updating items.
func (h *GraphQLHandler) buildUpdateItemInput() *graphql.InputObject {
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpdateItemInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name": &graphql.InputObjectFieldConfig{
				Type: graphql.NewNonNull(graphql.String),
			},
			"description": &graphql.InputObjectFieldConfig{
				Type: graphql.String,
			},
			"price": &graphql.InputObjectFieldConfig{
				Type: graphql.NewNonNull(graphql.Float),
			},
		},
	})
}

// buildQueryType defines the GraphQL root query type.
func (h *GraphQLHandler) buildQueryType(itemType *graphql.Object) *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"items": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(itemType))),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return h.resolveItems(p)
				},
			},
			"item": &graphql.Field{
				Type: itemType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.ID),
					},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return h.resolveItem(p)
				},
			},
		},
	})
}

// buildMutationType defines the GraphQL root mutation type.
func (h *GraphQLHandler) buildMutationType(
	itemType *graphql.Object,
	createInput *graphql.InputObject,
	updateInput *graphql.InputObject,
) *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createItem": &graphql.Field{
				Type: graphql.NewNonNull(itemType),
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(createInput),
					},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return h.resolveCreateItem(p)
				},
			},
			"updateItem": &graphql.Field{
				Type: graphql.NewNonNull(itemType),
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.ID),
					},
					"input": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(updateInput),
					},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return h.resolveUpdateItem(p)
				},
			},
			"deleteItem": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.ID),
					},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					return h.resolveDeleteItem(p)
				},
			},
		},
	})
}

// resolveItems handles the items query by listing all items from the store.
func (h *GraphQLHandler) resolveItems(p graphql.ResolveParams) (any, error) {
	ctx := p.Context

	items, err := h.store.List(ctx)
	if err != nil {
		h.logger.Error("failed to list items via GraphQL", zap.Error(err))
		return nil, fmt.Errorf("failed to retrieve items: %w", err)
	}

	h.logger.Debug("listed items via GraphQL", zap.Int("count", len(items)))

	// Convert to pointer slice so field resolvers receive *model.Item.
	ptrs := make([]*model.Item, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}

	return ptrs, nil
}

// resolveItem handles the item query by fetching a single item from the store.
func (h *GraphQLHandler) resolveItem(p graphql.ResolveParams) (any, error) {
	ctx := p.Context

	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("invalid item ID")
	}

	item, err := h.store.Get(ctx, id)
	if err != nil {
		return nil, h.mapStoreError(err, "get item")
	}

	h.logger.Debug("fetched item via GraphQL", zap.String("id", id))

	return item, nil
}

// resolveCreateItem handles the createItem mutation.
func (h *GraphQLHandler) resolveCreateItem(p graphql.ResolveParams) (any, error) {
	ctx := p.Context

	inputMap, ok := p.Args["input"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input")
	}

	input := h.parseItemInput(inputMap)

	if err := input.Validate(); err != nil {
		h.logger.Warn("GraphQL createItem validation failed", zap.Error(err))
		return nil, fmt.Errorf("validation error: %w", err)
	}

	item, err := h.store.Create(ctx, &input)
	if err != nil {
		return nil, h.mapStoreError(err, "create item")
	}

	h.logger.Info("created item via GraphQL", zap.String("id", item.ID))

	return item, nil
}

// resolveUpdateItem handles the updateItem mutation.
func (h *GraphQLHandler) resolveUpdateItem(p graphql.ResolveParams) (any, error) {
	ctx := p.Context

	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("invalid item ID")
	}

	inputMap, ok := p.Args["input"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input")
	}

	input := h.parseItemInput(inputMap)

	if err := input.Validate(); err != nil {
		h.logger.Warn("GraphQL updateItem validation failed", zap.String("id", id), zap.Error(err))
		return nil, fmt.Errorf("validation error: %w", err)
	}

	item, err := h.store.Update(ctx, id, &input)
	if err != nil {
		return nil, h.mapStoreError(err, "update item")
	}

	h.logger.Info("updated item via GraphQL", zap.String("id", id))

	return item, nil
}

// resolveDeleteItem handles the deleteItem mutation.
func (h *GraphQLHandler) resolveDeleteItem(p graphql.ResolveParams) (any, error) {
	ctx := p.Context

	id, ok := p.Args["id"].(string)
	if !ok || id == "" {
		return false, fmt.Errorf("invalid item ID")
	}

	if err := h.store.Delete(ctx, id); err != nil {
		return false, h.mapStoreError(err, "delete item")
	}

	h.logger.Info("deleted item via GraphQL", zap.String("id", id))

	return true, nil
}

// parseItemInput extracts item fields from a GraphQL input map.
func (h *GraphQLHandler) parseItemInput(inputMap map[string]any) model.Item {
	var item model.Item

	if name, ok := inputMap["name"].(string); ok {
		item.Name = name
	}

	if description, ok := inputMap["description"].(string); ok {
		item.Description = description
	}

	if price, ok := inputMap["price"].(float64); ok {
		item.Price = price
	}

	return item
}

// mapStoreError converts store errors into descriptive GraphQL errors.
func (h *GraphQLHandler) mapStoreError(err error, operation string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fmt.Errorf("item not found")
	case errors.Is(err, store.ErrInvalidID):
		return fmt.Errorf("invalid item ID")
	case errors.Is(err, store.ErrAlreadyExists):
		return fmt.Errorf("item already exists")
	default:
		h.logger.Error("GraphQL store operation failed",
			zap.String("operation", operation),
			zap.Error(err),
		)
		return fmt.Errorf("internal server error")
	}
}
