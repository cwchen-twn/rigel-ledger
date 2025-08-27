package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")

	ErrUsernameRequired     = errors.New("username is required")
	ErrEmailRequired        = errors.New("email is required")
	ErrPasswordHashRequired = errors.New("password hash is required")
	ErrFirstNameRequired    = errors.New("first name is required")
	ErrLastNameRequired     = errors.New("last name is required")
)

type User struct {
	Username      string       `db:"username" json:"username"`
	Email         string       `db:"email" json:"email"`
	PasswordHash  string       `db:"password_hash" json:"password_hash"`
	FirstName     string       `db:"first_name" json:"first_name"`
	LastName      string       `db:"last_name" json:"last_name"`
	IsActive      bool         `db:"is_active" json:"is_active"`
	EmailVerified bool         `db:"email_verified" json:"email_verified"`
	LastLogin     sql.NullTime `db:"last_login" json:"last_login"`
	MainCountry   string       `db:"main_country" json:"main_country"`
	MainLanguage  string       `db:"main_language" json:"main_language"`
	MainCurrency  string       `db:"main_currency" json:"main_currency"`
	CreatedAt     time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt     sql.NullTime `db:"updated_at" json:"updated_at"`
}

func FindByUserName(username string, conn *sqlx.DB) (*User, error) {
	query := `
		SELECT
			username,
			email,
			password_hash,
			first_name,
			last_name,
			is_active,
			email_verified,
			last_login,
			main_country,
			main_language,
			main_currency,
			created_at,
			updated_at
		FROM
			users
		WHERE
			username = $1
	`

	var user User
	if err := conn.Get(&user, query, username); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (u *User) validateBeforeCreate() error {
	if u.Username == "" {
		return ErrUsernameRequired
	}
	if u.Email == "" {
		return ErrEmailRequired
	}
	if u.PasswordHash == "" {
		return ErrPasswordHashRequired
	}
	if u.FirstName == "" {
		return ErrFirstNameRequired
	}
	if u.LastName == "" {
		return ErrLastNameRequired
	}

	return nil
}

func (u *User) Create(conn *sqlx.DB) error {
	if err := u.validateBeforeCreate(); err != nil {
		return err
	}

	query := `
		INSERT INTO users (
			username,
			email,
			password_hash,
			first_name,
			last_name,
			main_country,
			main_language,
			main_currency
		) VALUES (
			:username,
			:email,
			:password_hash,
			UPPER(TRIM(:first_name)),
			UPPER(TRIM(:last_name)),
			:main_country,
			:main_language,
			:main_currency
		)
	`

	_, err := conn.NamedExec(query, u)
	if err != nil {
		return err
	}

	return nil
}

func (u *User) ValidatePassword(password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}

	return nil
}

func (u *User) ToJSONString() (string, error) {
	json, err := json.Marshal(u)
	if err != nil {
		return "", err
	}

	return string(json), nil
}
