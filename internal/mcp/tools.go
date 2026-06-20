package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ananthakumaran/paisa/internal/server"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"gorm.io/gorm"
)

type handlerFunc func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)

func jsonText(v any) (*mcpgo.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: string(b)}},
	}, nil
}

// getDashboardHandler returns a handler that fetches the full dashboard summary.
func getDashboardHandler(db *gorm.DB) handlerFunc {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return jsonText(server.GetDashboard(db))
	}
}

// getNetworthHandler returns a handler that fetches networth history.
func getNetworthHandler(db *gorm.DB) handlerFunc {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return jsonText(server.GetNetworth(db))
	}
}

// getAllocationHandler returns a handler that fetches asset allocation data.
func getAllocationHandler(db *gorm.DB) handlerFunc {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return jsonText(server.GetAllocation(db))
	}
}

// getInsightsHandler returns a handler that fetches spending insights.
func getInsightsHandler(db *gorm.DB) handlerFunc {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return jsonText(server.GetInsights(db))
	}
}

// getBudgetHandler returns a handler that fetches budget data.
// An optional "month" argument in "YYYY-MM" format is accepted for compatibility
// but the underlying GetBudget function uses current budget data.
func getBudgetHandler(db *gorm.DB) handlerFunc {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return jsonText(server.GetBudget(db))
	}
}
