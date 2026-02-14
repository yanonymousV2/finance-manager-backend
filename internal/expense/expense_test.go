package expense

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanonymousV2/finance-manager-backend/internal/db"
)

func setupExpenseTestDB(t *testing.T) *db.DB {
	dbURL := "postgres://postgres:postgres@localhost:5432/finance_manager_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)

	// Clean up tables
	_, err = pool.Exec(context.Background(), "TRUNCATE users, groups, group_members, expenses, expense_splits CASCADE")
	require.NoError(t, err)

	return &db.DB{Pool: pool}
}

func createTestUser(t *testing.T, testDB *db.DB, email string) uuid.UUID {
	userID := uuid.New()
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)",
		userID, email, "hashedpassword")
	require.NoError(t, err)
	return userID
}

func createTestGroup(t *testing.T, testDB *db.DB, creatorID uuid.UUID) uuid.UUID {
	groupID := uuid.New()
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO groups (id, name, created_by) VALUES ($1, $2, $3)",
		groupID, "Test Group", creatorID)
	require.NoError(t, err)

	// Add creator as member
	_, err = testDB.Pool.Exec(context.Background(),
		"INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)",
		groupID, creatorID)
	require.NoError(t, err)

	return groupID
}

func TestCreateExpense_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testDB := setupExpenseTestDB(t)
	defer testDB.Close()

	userID := createTestUser(t, testDB, "test@example.com")
	groupID := createTestGroup(t, testDB, userID)
	userID2 := createTestUser(t, testDB, "test2@example.com")

	// Add second user to group
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)",
		groupID, userID2)
	require.NoError(t, err)

	tests := []struct {
		name           string
		requestBody    CreateExpenseRequest
		expectedStatus int
		expectError    bool
	}{
		{
			name: "valid expense",
			requestBody: CreateExpenseRequest{
				GroupID:     groupID,
				Description: "Test Expense",
				TotalAmount: "100.0",
				Splits: []CreateExpenseSplitRequest{
					{UserID: userID, Amount: "50.0"},
					{UserID: userID2, Amount: "50.0"},
				},
			},
			expectedStatus: 201,
			expectError:    false,
		},
		{
			name: "splits sum mismatch",
			requestBody: CreateExpenseRequest{
				GroupID:     groupID,
				Description: "Test Expense",
				TotalAmount: "100.0",
				Splits: []CreateExpenseSplitRequest{
					{UserID: userID, Amount: "30.0"},
					{UserID: userID2, Amount: "50.0"},
				},
			},
			expectedStatus: 400,
			expectError:    true,
		},
		{
			name: "duplicate user in splits",
			requestBody: CreateExpenseRequest{
				GroupID:     groupID,
				Description: "Test Expense",
				TotalAmount: "100.0",
				Splits: []CreateExpenseSplitRequest{
					{UserID: userID, Amount: "50.0"},
					{UserID: userID, Amount: "50.0"},
				},
			},
			expectedStatus: 400,
			expectError:    true,
		},
		{
			name: "non-member in splits",
			requestBody: CreateExpenseRequest{
				GroupID:     groupID,
				Description: "Test Expense",
				TotalAmount: "100.0",
				Splits: []CreateExpenseSplitRequest{
					{UserID: userID, Amount: "50.0"},
					{UserID: uuid.New(), Amount: "50.0"}, // Non-member
				},
			},
			expectedStatus: 400,
			expectError:    true,
		},
		{
			name: "negative split amount",
			requestBody: CreateExpenseRequest{
				GroupID:     groupID,
				Description: "Test Expense",
				TotalAmount: "100.0",
				Splits: []CreateExpenseSplitRequest{
					{UserID: userID, Amount: "-10.0"},
					{UserID: userID2, Amount: "110.0"},
				},
			},
			expectedStatus: 400,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set user ID in context (simulate middleware)
			c.Set("user_id", userID)

			body, _ := json.Marshal(tt.requestBody)
			c.Request = httptest.NewRequest("POST", "/expenses", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			CreateExpense(c, testDB)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectError {
				assert.Contains(t, response, "error")
			} else {
				assert.Contains(t, response, "id")
				assert.Contains(t, response, "description")
			}
		})
	}
}
