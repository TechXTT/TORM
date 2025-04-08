package db_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	db "github.com/TechXTT/TORM"
	"github.com/stretchr/testify/assert"
)

type Users struct {
	Id       int
	Username string
}

// TestNewDB ensures that NewDB correctly initializes a database connection.
func TestNewDB(t *testing.T) {
	// Mock database connection
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	assert.NoError(t, err)
	defer mockDB.Close()

	// Expect a Ping to be successful
	mock.ExpectPing()

	// Create the database instance
	testDB := &db.DB{Conn: mockDB}

	err = testDB.Conn.Ping()
	assert.NoError(t, err)
}

// TestQuery checks if the Query function executes without errors.
func TestSelectQuery(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "TechXT")

	mock.ExpectQuery(`SELECT \* FROM users`).WillReturnRows(rows)

	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Create a new QueryBuilder instance
	qb := testDB.Query("users")

	// Destination slice
	var users []Users

	// Execute the query
	err = qb.Select(&users)
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())

	// Ensure the correct data was returned
	assert.Equal(t, 1, users[0].Id)
	assert.Equal(t, "TechXT", users[0].Username)
}

func TestWhereQuery(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "TechXT")

	mock.ExpectQuery(`SELECT \* FROM users WHERE id = \'1\'`).WillReturnRows(rows)

	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Create a new QueryBuilder instance
	qb := testDB.Query("users").Where("id = ?", 1)

	// Destination slice
	var users []Users

	// Execute the query
	err = qb.Select(&users)
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())

	// Ensure the correct data was returned
	assert.Equal(t, 1, users[0].Id)
	assert.Equal(t, "TechXT", users[0].Username)
}

func TestInsertQuery(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	expectedInsert := `INSERT INTO users \(id,username\) VALUES \(\?,\?\);`

	prep := mock.ExpectPrepare(expectedInsert)

	prep.ExpectExec().WithArgs(1, "TechXT").WillReturnResult(sqlmock.NewResult(1, 1))
	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Create a new QueryBuilder instance
	qb := testDB.Query("users")

	user := &Users{
		Id:       1,
		Username: "TechXT",
	}

	// Execute the insert query
	err = qb.Insert(user)
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateQuery(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	// Mock expected query execution
	expectedInsert := `UPDATE users SET id = \?,username = \? WHERE id = \?;`

	prep := mock.ExpectPrepare(expectedInsert)

	prep.ExpectExec().WithArgs(1, "TechXTT", 1).WillReturnResult(sqlmock.NewResult(1, 1))
	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Create a new QueryBuilder instance
	qb := testDB.Query("users").Where("id = ?", 1)

	// Execute the update query
	err = qb.Update(&Users{Id: 1, Username: "TechXTT"})
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAutoMigrate(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS users \(id INTEGER\,username TEXT\)`).WillReturnResult(sqlmock.NewResult(0, 0))

	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Execute the query
	err = testDB.AutoMigrate(&Users{})
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
