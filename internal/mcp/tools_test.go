package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ananthakumaran/paisa/internal/model"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// SQLite in-memory with shared cache requires a single connection so all
	// goroutines see the same database after AutoMigrate.
	sqlDB.SetMaxOpenConns(1)
	model.AutoMigrate(db)
	return db
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return text.Text
}

func assertValidJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	err := json.Unmarshal([]byte(s), &m)
	require.NoError(t, err, "expected valid JSON, got: %s", s)
	return m
}

func TestGetDashboard(t *testing.T) {
	db := newTestDB(t)
	h := getDashboardHandler(db)
	out := callTool(t, h, nil)
	m := assertValidJSON(t, out)
	assert.Contains(t, m, "networth")
	assert.Contains(t, m, "expenses")
	assert.Contains(t, m, "budget")
}

func TestGetNetworth(t *testing.T) {
	db := newTestDB(t)
	h := getNetworthHandler(db)
	out := callTool(t, h, nil)
	assertValidJSON(t, out)
}

func TestGetAllocation(t *testing.T) {
	db := newTestDB(t)
	h := getAllocationHandler(db)
	out := callTool(t, h, nil)
	assertValidJSON(t, out)
}

func TestGetInsights(t *testing.T) {
	db := newTestDB(t)
	h := getInsightsHandler(db)
	out := callTool(t, h, nil)
	assertValidJSON(t, out)
}

func TestGetBudget_NoParam(t *testing.T) {
	db := newTestDB(t)
	h := getBudgetHandler(db)
	out := callTool(t, h, nil)
	assertValidJSON(t, out)
}

func TestGetBudget_WithMonth(t *testing.T) {
	db := newTestDB(t)
	h := getBudgetHandler(db)
	out := callTool(t, h, map[string]any{"month": "2026-01"})
	assertValidJSON(t, out)
}
