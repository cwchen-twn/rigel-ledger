// rigel-ledger-cli administers users from inside the running container. The
// Administration page (invitations) is the usual way in; this is for the first
// admin and for recovery:
//
//	rigel-ledger-cli create-user -u alice -e alice@example.com --display-name Alice
//	rigel-ledger-cli reset-password -u alice
//	rigel-ledger-cli set-admin -u alice --admin=true
//	rigel-ledger-cli reset-mfa -u alice       # lost every second factor
//
// A password not given with -p is read from stdin, so it stays out of the
// shell history and the process list. A created user still walks the
// first-login wizard (and verifies the address) at the first sign-in.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/cwchen-twn/rigel-ledger/internal"
	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/identity"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// Set at build time via -ldflags "-X main.version=...".
var version = "dev"

func usage() {
	fmt.Fprintf(os.Stderr, `rigel-ledger-cli %s

Usage:
  rigel-ledger-cli create-user    -u USER -e EMAIL [-p PASS] [--display-name N] [--language en|zh|es] [--currency USD] [--timezone UTC] [--admin]
  rigel-ledger-cli reset-password -u USER [-p PASS]
  rigel-ledger-cli set-admin      -u USER --admin=true|false
  rigel-ledger-cli reset-mfa      -u USER    remove every second factor and session (break-glass)

Configuration comes from the same environment (or .env) as the server.
`, version)
}

func readPassword(given string) string {
	if given != "" {
		return given
	}
	fmt.Fprint(os.Stderr, "Password: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	cfg, err := internal.LoadConfig()
	if err != nil {
		die(err)
	}
	if cfg.AppVersion == "" {
		cfg.AppVersion = version
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store, err := internal.OpenStore(cfg, logger)
	if err != nil {
		die(err)
	}
	defer store.Close()
	svc := ledger.NewService(store)
	ctx := context.Background()

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	username := fs.StringP("username", "u", "", "username (lowercase)")
	password := fs.StringP("password", "p", "", "password; read from stdin when omitted")

	switch cmd {
	case "create-user":
		email := fs.StringP("email", "e", "", "email")
		display := fs.String("display-name", "", "display name")
		language := fs.String("language", "en", "en, zh or es")
		currency := fs.String("currency", "USD", "display currency")
		timezone := fs.String("timezone", "UTC", "IANA time zone")
		admin := fs.Bool("admin", false, "make the user an administrator")
		_ = fs.Parse(args)
		u, err := svc.CreateUser(ctx, ledger.NewUser{
			Username: *username, Email: *email, Password: readPassword(*password), DisplayName: *display,
			Language: *language, DisplayCurrency: *currency, Timezone: *timezone, IsAdmin: *admin,
		})
		if err != nil {
			die(err)
		}
		fmt.Printf("created user %s (id %d)\n", u.Username, u.ID)

	case "reset-password":
		_ = fs.Parse(args)
		if err := svc.ResetPassword(ctx, *username, readPassword(*password)); err != nil {
			die(err)
		}
		fmt.Printf("password reset for %s; all their sessions were signed out\n", *username)

	case "set-admin":
		admin := fs.Bool("admin", true, "true to grant, false to revoke")
		_ = fs.Parse(args)
		if err := svc.SetAdmin(ctx, *username, *admin); err != nil {
			die(err)
		}
		fmt.Printf("%s admin=%v\n", *username, *admin)

	case "reset-mfa":
		_ = fs.Parse(args)
		u, err := store.GetUserByUsername(ctx, strings.ToLower(*username))
		if err != nil {
			die(fmt.Errorf("no user %q", *username))
		}
		ids := identity.New(identity.Options{Store: store, Ledger: svc, Auth: auth.NewManager(store, cfg.SessionTTL, true), Logger: logger})
		if err := ids.ResetMFA(ctx, 0, "cli", u.ID); err != nil {
			die(err)
		}
		fmt.Printf("%s: every second factor, recovery code and session removed; the next sign-in enrols again\n", u.Username)

	default:
		usage()
		os.Exit(2)
	}
}
