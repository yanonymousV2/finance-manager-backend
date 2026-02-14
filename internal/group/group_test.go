package group

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanonymousV2/finance-manager-backend/internal/db"
)

func setupBalanceTestDB(t *testing.T) *db.DB {
	dbURL := "postgres://postgres:postgres@localhost:5432/finance_manager_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)

	// Clean up tables
	_, err = pool.Exec(context.Background(), "TRUNCATE users, groups, group_members, expenses, expense_splits, settlements CASCADE")
	require.NoError(t, err)

	return &db.DB{Pool: pool}
}

func createBalanceTestUser(t *testing.T, testDB *db.DB, email string) uuid.UUID {
	userID := uuid.New()
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)",
		userID, email, "hashedpassword")
	require.NoError(t, err)
	return userID
}

func createBalanceTestGroup(t *testing.T, testDB *db.DB, creatorID uuid.UUID) uuid.UUID {
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

func addGroupMember(t *testing.T, testDB *db.DB, groupID, userID uuid.UUID) {
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)",
		groupID, userID)
	require.NoError(t, err)
}

func createExpense(t *testing.T, testDB *db.DB, groupID, paidBy uuid.UUID, total decimal.Decimal, splits map[uuid.UUID]decimal.Decimal) uuid.UUID {
	expenseID := uuid.New()
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO expenses (id, group_id, description, total_amount, paid_by) VALUES ($1, $2, $3, $4, $5)",
		expenseID, groupID, "Test Expense", total, paidBy)
	require.NoError(t, err)

	for userID, amount := range splits {
		_, err := testDB.Pool.Exec(context.Background(),
			"INSERT INTO expense_splits (expense_id, user_id, amount) VALUES ($1, $2, $3)",
			expenseID, userID, amount)
		require.NoError(t, err)
	}

	return expenseID
}

func createSettlement(t *testing.T, testDB *db.DB, groupID, fromUser, toUser uuid.UUID, amount decimal.Decimal) uuid.UUID {
	settlementID := uuid.New()
	_, err := testDB.Pool.Exec(context.Background(),
		"INSERT INTO settlements (id, group_id, from_user, to_user, amount) VALUES ($1, $2, $3, $4, $5)",
		settlementID, groupID, fromUser, toUser, amount)
	require.NoError(t, err)
	return settlementID
}

func TestBalanceCalculation(t *testing.T) {
	testDB := setupBalanceTestDB(t)
	defer testDB.Close()

	userA := createBalanceTestUser(t, testDB, "a@example.com")
	userB := createBalanceTestUser(t, testDB, "b@example.com")
	userC := createBalanceTestUser(t, testDB, "c@example.com")

	groupID := createBalanceTestGroup(t, testDB, userA)
	addGroupMember(t, testDB, groupID, userB)
	addGroupMember(t, testDB, groupID, userC)

	tests := []struct {
		name     string
		setup    func()
		expected map[uuid.UUID]decimal.Decimal
	}{
		{
			name: "simple expense - A pays 100, split equally",
			setup: func() {
				createExpense(t, testDB, groupID, userA, decimal.NewFromFloat(100),
					map[uuid.UUID]decimal.Decimal{
						userA: decimal.NewFromFloat(33.33),
						userB: decimal.NewFromFloat(33.33),
						userC: decimal.NewFromFloat(33.34),
					})
			},
			expected: map[uuid.UUID]decimal.Decimal{
				userA: decimal.NewFromFloat(100).Sub(decimal.NewFromFloat(33.33)), // ~66.67
				userB: decimal.NewFromFloat(0).Sub(decimal.NewFromFloat(33.33)),   // ~-33.33
				userC: decimal.NewFromFloat(0).Sub(decimal.NewFromFloat(33.34)),   // ~-33.34
			},
		},
		{
			name: "settlement reduces balance",
			setup: func() {
				createExpense(t, testDB, groupID, userA, decimal.NewFromFloat(60),
					map[uuid.UUID]decimal.Decimal{
						userA: decimal.NewFromFloat(20),
						userB: decimal.NewFromFloat(20),
						userC: decimal.NewFromFloat(20),
					})
				createSettlement(t, testDB, groupID, userB, userA, decimal.NewFromFloat(20))
			},
			expected: map[uuid.UUID]decimal.Decimal{
				userA: decimal.NewFromFloat(60).Sub(decimal.NewFromFloat(20)),                              // 40
				userB: decimal.NewFromFloat(0).Sub(decimal.NewFromFloat(20)).Add(decimal.NewFromFloat(20)), // 0
				userC: decimal.NewFromFloat(0).Sub(decimal.NewFromFloat(20)),                               // -20
			},
		},
		{
			name: "multiple expenses and settlements",
			setup: func() {
				// Expense 1: A pays 90, split equally
				createExpense(t, testDB, groupID, userA, decimal.NewFromFloat(90),
					map[uuid.UUID]decimal.Decimal{
						userA: decimal.NewFromFloat(30),
						userB: decimal.NewFromFloat(30),
						userC: decimal.NewFromFloat(30),
					})
				// Expense 2: B pays 60, split equally
				createExpense(t, testDB, groupID, userB, decimal.NewFromFloat(60),
					map[uuid.UUID]decimal.Decimal{
						userA: decimal.NewFromFloat(20),
						userB: decimal.NewFromFloat(20),
						userC: decimal.NewFromFloat(20),
					})
				// Settlement: C pays A 10
				createSettlement(t, testDB, groupID, userC, userA, decimal.NewFromFloat(10))
			},
			expected: map[uuid.UUID]decimal.Decimal{
				userA: decimal.NewFromFloat(90).Sub(decimal.NewFromFloat(30)).Sub(decimal.NewFromFloat(20)).Add(decimal.NewFromFloat(10)), // 90 - 30 - 20 + 10 = 50
				userB: decimal.NewFromFloat(60).Sub(decimal.NewFromFloat(20)).Sub(decimal.NewFromFloat(30)),                               // 60 - 20 - 30 = 10
				userC: decimal.NewFromFloat(0).Sub(decimal.NewFromFloat(30)).Sub(decimal.NewFromFloat(20)).Sub(decimal.NewFromFloat(10)),  // -30 -20 -10 = -60
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up for each test
			_, err := testDB.Pool.Exec(context.Background(), "TRUNCATE expenses, expense_splits, settlements CASCADE")
			require.NoError(t, err)

			tt.setup()

			// Calculate balances by simulating the logic
			balances := make(map[uuid.UUID]decimal.Decimal)
			balances[userA] = decimal.Zero
			balances[userB] = decimal.Zero
			balances[userC] = decimal.Zero

			// Add from expenses
			expRows, err := testDB.Pool.Query(context.Background(),
				"SELECT paid_by, total_amount FROM expenses WHERE group_id = $1", groupID)
			require.NoError(t, err)
			defer expRows.Close()

			for expRows.Next() {
				var paidBy uuid.UUID
				var total decimal.Decimal
				err := expRows.Scan(&paidBy, &total)
				require.NoError(t, err)
				balances[paidBy] = balances[paidBy].Add(total)
			}

			// Subtract splits
			splitRows, err := testDB.Pool.Query(context.Background(),
				"SELECT es.user_id, es.amount FROM expense_splits es JOIN expenses e ON es.expense_id = e.id WHERE e.group_id = $1", groupID)
			require.NoError(t, err)
			defer splitRows.Close()

			for splitRows.Next() {
				var uid uuid.UUID
				var amt decimal.Decimal
				err := splitRows.Scan(&uid, &amt)
				require.NoError(t, err)
				balances[uid] = balances[uid].Sub(amt)
			}

			// Adjust settlements
			settRows, err := testDB.Pool.Query(context.Background(),
				"SELECT from_user, to_user, amount FROM settlements WHERE group_id = $1", groupID)
			require.NoError(t, err)
			defer settRows.Close()

			for settRows.Next() {
				var from, to uuid.UUID
				var amt decimal.Decimal
				err := settRows.Scan(&from, &to, &amt)
				require.NoError(t, err)
				balances[from] = balances[from].Sub(amt)
				balances[to] = balances[to].Add(amt)
			}

			// Check expectations (allowing small decimal differences)
			for userID, expectedBalance := range tt.expected {
				actual := balances[userID]
				diff := expectedBalance.Sub(actual).Abs()
				assert.True(t, diff.LessThan(decimal.NewFromFloat(0.01)),
					"Balance for user %s: expected %s, got %s", userID, expectedBalance, actual)
			}
		})
	}
}
