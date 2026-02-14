-- Drop indexes
DROP INDEX IF EXISTS idx_personal_expenses_user_date_range;
DROP INDEX IF EXISTS idx_personal_expenses_expense_date;
DROP INDEX IF EXISTS idx_personal_expenses_category_id;
DROP INDEX IF EXISTS idx_personal_expenses_user_id;
DROP INDEX IF EXISTS idx_monthly_budgets_month_year;
DROP INDEX IF EXISTS idx_monthly_budgets_user_id;
DROP INDEX IF EXISTS idx_expense_categories_user_id;

-- Drop tables
DROP TABLE IF EXISTS personal_expenses;
DROP TABLE IF EXISTS monthly_budgets;
DROP TABLE IF EXISTS expense_categories;
