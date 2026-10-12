// Command seed-users creates the accounts listed in a YAML file through the
// real identity + account-profile flow, for local testing:
//
//  1. register    -> identity.accounts (status=pending) + a verification code
//  2. read the code straight from the DB (dev only - stands in for the email)
//  3. verify      -> the account turns active and raises identity.account.verified
//  4. outbox-publisher ships that event over NATS and account-profile creates
//     the (empty) profile and default settings
//  5. login, then fill in the profile and settings from the YAML
//
// It is safe to re-run: an existing account (matched by email) only gets the
// steps it is missing, and its profile and settings are set again from the file.
//
// Requires gateway, identity, account-profile, outbox-publisher, nats and db
// running (task docker:up). See users.example.yaml for the file format.
//
//	go run ./scripts/seed-users -file scripts/seed-users/users.example.yaml
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"
)

type options struct {
	file           string
	gatewayURL     string
	databaseURL    string
	profileTimeout time.Duration
}

func main() {
	opts := options{}
	flag.StringVar(&opts.file, "file", "", "YAML file with the users to create (required)")
	flag.StringVar(&opts.gatewayURL, "gateway", envOr("GATEWAY_URL", "http://localhost:8080"), "gateway base URL (env GATEWAY_URL)")
	flag.StringVar(&opts.databaseURL, "db", envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/rekreativko?sslmode=disable"), "Postgres URL, used to read verification codes (env DATABASE_URL)")
	flag.DurationVar(&opts.profileTimeout, "profile-timeout", 30*time.Second, "how long to wait for account-profile to create a new user's profile")
	flag.Parse()

	if opts.file == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	file, err := LoadSeedFile(opts.file)
	if err != nil {
		return err
	}

	db, err := pgx.Connect(ctx, opts.databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close(ctx)

	s := &seeder{
		api:            newAPIClient(opts.gatewayURL),
		db:             db,
		profileTimeout: opts.profileTimeout,
	}

	fmt.Printf("Seeding %d users against %s\n\n", len(file.Users), opts.gatewayURL)

	results := make([]result, 0, len(file.Users))
	failed := 0
	for _, u := range file.Users {
		r := s.seed(ctx, u)
		if r.err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "x %s: %v\n", u.Email, r.err)
		} else {
			fmt.Printf("+ %s: %s\n", u.Email, r.status)
		}
		results = append(results, r)
	}

	fmt.Println()
	printResults(results)

	if failed > 0 {
		return fmt.Errorf("%d of %d users failed", failed, len(file.Users))
	}
	return nil
}

type result struct {
	user      User
	accountID string
	status    string
	err       error
}

type seeder struct {
	api            *apiClient
	db             *pgx.Conn
	profileTimeout time.Duration
}

// seed runs the steps the account is still missing; a failure stops this user
// only, the caller carries on with the next.
func (s *seeder) seed(ctx context.Context, u User) result {
	r := result{user: u}

	accountID, status, err := s.existingAccount(ctx, u.Email)
	if err != nil {
		r.err = err
		return r
	}

	if accountID == "" {
		accountID, err = s.api.Register(ctx, u.Email, u.Password)
		if err != nil {
			r.err = err
			return r
		}
		r.status = "registered"
	} else {
		r.status = "existed"
	}
	r.accountID = accountID

	if status != "active" {
		code, err := s.verificationCode(ctx, accountID)
		if err != nil {
			r.err = err
			return r
		}
		if err := s.api.VerifyAccount(ctx, code); err != nil {
			r.err = err
			return r
		}
		r.status += ", verified"
	}

	token, err := s.api.Login(ctx, u.Email, u.Password)
	if err != nil {
		r.err = fmt.Errorf("%w (an existing account keeps its old password)", err)
		return r
	}

	if err := s.waitForProfile(ctx, accountID); err != nil {
		r.err = err
		return r
	}

	if err := s.api.UpdateProfile(ctx, token, u.Profile); err != nil {
		r.err = err
		return r
	}
	r.status += ", profile set"

	if len(u.Settings) > 0 {
		if err := s.api.UpdateSettings(ctx, token, u.Settings); err != nil {
			r.err = err
			return r
		}
		r.status += ", settings set"
	}

	return r
}

// existingAccount returns the account's ID and status, or "" if there is none.
func (s *seeder) existingAccount(ctx context.Context, email string) (id, status string, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT id::text, status
		FROM identity.accounts
		WHERE email = $1 AND deleted_at IS NULL
	`, email).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("look up account: %w", err)
	}
	return id, status, nil
}

func (s *seeder) verificationCode(ctx context.Context, accountID string) (string, error) {
	var code string
	err := s.db.QueryRow(ctx, `
		SELECT code
		FROM identity.verification_codes
		WHERE account_id = $1 AND used_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, accountID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("no unused verification code for the account")
	}
	if err != nil {
		return "", fmt.Errorf("read verification code: %w", err)
	}
	return code, nil
}

// waitForProfile polls for the profile account-profile creates asynchronously
// once the account is verified.
func (s *seeder) waitForProfile(ctx context.Context, accountID string) error {
	deadline := time.Now().Add(s.profileTimeout)
	for {
		var exists bool
		err := s.db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM account_profile.profiles WHERE account_id = $1)`,
			accountID,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check profile: %w", err)
		}
		if exists {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("profile not created within %s - are outbox-publisher, nats and account-profile running?", s.profileTimeout)
		}
		time.Sleep(time.Second)
	}
}

func printResults(results []result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tPASSWORD\tNICKNAME\tACCOUNT_ID\tRESULT")
	for _, r := range results {
		outcome := "ok"
		if r.err != nil {
			outcome = "FAILED"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.user.Email, r.user.Password, r.user.Profile.Nickname, r.accountID, outcome)
	}
	_ = w.Flush()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
