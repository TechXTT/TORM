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
func TestSelect(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "TechXT")

	mock.ExpectQuery(`SELECT \* FROM users`).WillReturnRows(rows)

	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Create a slice of TestUser structs
	var users []Users

	// Execute the query
	err = testDB.Select(&users)
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())

	// Check if users has the TechXT user
	assert.Len(t, users, 1)
	assert.Equal(t, "TechXT", users[0].Username)
}

func TestAutoMigrate(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	// Mock expected query execution
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS users \(id TEXT\,username TEXT\)`).WillReturnResult(sqlmock.NewResult(0, 0))

	// Create DB instance
	testDB := &db.DB{Conn: mockDB}

	// Execute the query
	err = testDB.AutoMigrate(&Users{})
	assert.NoError(t, err)

	// Ensure expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
