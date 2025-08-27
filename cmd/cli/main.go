package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/jmoiron/sqlx"
	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/bcrypt"

	"github.com/cwc1222/rigelledger/internal"
	"github.com/cwc1222/rigelledger/internal/models"
)

var (
	command Command
)

func validateUser(u *models.User, password string) error {
	if u.Username == "" {
		return fmt.Errorf("username is required")
	}
	if u.Email == "" {
		return fmt.Errorf("email is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	if u.FirstName == "" {
		return fmt.Errorf("first name is required")
	}
	if u.LastName == "" {
		return fmt.Errorf("last name is required")
	}

	if u.MainCountry == "" {
		u.MainCountry = "US"
	}
	if u.MainLanguage == "" {
		u.MainLanguage = "en"
	}
	if u.MainCurrency == "" {
		u.MainCurrency = "USD"
	}

	return nil
}

func createUserCommand(conn *sqlx.DB) {
	var user models.User
	var password string

	cuFlags := flag.NewFlagSet("create-user", flag.ExitOnError)
	cuFlags.StringVarP(&user.Username, "username", "u", "", "Username")
	cuFlags.StringVarP(&user.Email, "email", "e", "", "Email")
	cuFlags.StringVarP(&password, "password", "p", "", "Password")
	cuFlags.StringVarP(&user.FirstName, "first-name", "f", "", "First Name")
	cuFlags.StringVarP(&user.LastName, "last-name", "l", "", "Last Name")
	cuFlags.Parse(os.Args[2:])

	if err := validateUser(&user, password); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	user.PasswordHash = string(passwordHash)

	json, err := user.ToJSONString()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(json)

	if err := user.Create(conn); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("User created successfully")
}

func main() {

	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Println("Failed to load config", err)
		os.Exit(1)
	}

	logger := internal.NewLogger(internal.LoggerConfig{
		AppName:    cfg.AppName,
		AppVersion: cfg.AppVersion,
		AppEnv:     cfg.AppEnv,
		LogLevel:   cfg.GetLogLevel(),
		Handler:    slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.GetLogLevel()}),
	})

	db, err := internal.NewPostgres(cfg, logger)
	if err != nil {
		logger.Error("Failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	rootFlags := flag.NewFlagSet("root", flag.ExitOnError)
	rootFlags.VarP(&command, "command", "c", "Run pre-defined commands")
	rootFlags.Parse(os.Args[1:2])

	if command.Value == "" {
		fmt.Println("No command provided")
		os.Exit(1)
	}

	switch command.Value {
	case "create-user":
		createUserCommand(db.GetDB())
	default:
		fmt.Println("Invalid command")
	}
}
