//go:build integration
// +build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/arinbalyan/jobhunter/internal/db"
	"github.com/arinbalyan/jobhunter/internal/migrations"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestWithPostgresContainer runs database operations against a real PostgreSQL.
// Run with: go test -tags=integration ./tests/db/ -v -count=1
func TestWithPostgresContainer(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "jobhunter_test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start postgres: %v", err)
	}
	defer pg.Terminate(ctx)

	host, err := pg.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}
	port, err := pg.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get port: %v", err)
	}

	dbURL := "postgres://test:test@" + host + ":" + port.Port() + "/jobhunter_test?sslmode=disable"

	// Run migrations
	if err := migrations.Run(dbURL); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Connect
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer pool.Close()

	t.Run("InsertJob", func(t *testing.T) {
		id, isNew, err := pool.InsertJobFull(ctx, &db.FullJobRecord{
			JobID:      "test-1",
			Title:      "Software Engineer",
			Company:    "Acme Corp",
			JobURL:     "https://example.com/job/1",
			Location:   "Remote",
			IsRemote:   true,
			Source:     "linkedin",
			Status:     "new",
		})
		if err != nil {
			t.Fatalf("InsertJobFull failed: %v", err)
		}
		if !isNew {
			t.Fatal("Expected isNew=true for new job")
		}
		if id <= 0 {
			t.Fatal("Expected valid id")
		}

		// Insert again — should not be new (duplicate URL)
		_, isNew, err = pool.InsertJobFull(ctx, &db.FullJobRecord{
			JobID:  "test-1",
			Title:  "Software Engineer",
			Company: "Acme Corp",
			JobURL: "https://example.com/job/1",
			Source: "linkedin",
			Status: "new",
		})
		if err != nil {
			t.Fatalf("InsertJobFull duplicate failed: %v", err)
		}
		if isNew {
			t.Fatal("Expected isNew=false for duplicate URL")
		}
	})

	t.Run("InsertJobWithNewFields", func(t *testing.T) {
		id, isNew, err := pool.InsertJobFull(ctx, &db.FullJobRecord{
			JobID:              "test-2",
			Title:              "Backend Engineer",
			Company:            "Beta Inc",
			JobURL:             "https://example.com/job/2",
			Location:           "New York",
			Source:             "indeed",
			Status:             "pending",
			Domain:             "beta.com",
			CompanyDescription: "A software company",
			QualityScore:       85,
			ExperienceRange:    "3-5 years",
			CompanyIndustry:    "Technology",
		})
		if err != nil {
			t.Fatalf("InsertJobFull failed: %v", err)
		}
		if !isNew {
			t.Fatal("Expected isNew=true")
		}
		if id <= 0 {
			t.Fatal("Expected valid id")
		}
	})

	t.Run("RecordRunLog", func(t *testing.T) {
		err := pool.RecordRunLog(ctx, "scrape", "completed", 10, 5, 3, 0, 0, 1200, "")
		if err != nil {
			t.Fatalf("RecordRunLog failed: %v", err)
		}
	})

	t.Run("GetTodaySentCount", func(t *testing.T) {
		count, err := pool.GetTodaySentCount(ctx)
		if err != nil {
			t.Fatalf("GetTodaySentCount failed: %v", err)
		}
		if count != 0 {
			t.Fatalf("Expected 0 sent today, got %d", count)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()
		// Should timeout quickly
		_, err := pool.GetTodaySentCount(timeoutCtx)
		if err == nil {
			t.Log("Query succeeded despite short timeout (may be instant)")
		}
	})
}
