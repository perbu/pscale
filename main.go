package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Import pgx stdlib driver for goose
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

const (
	totalRows = 10_000_000
)

var batchSizes = []int{100, 1000, 10_000, 100_000, 1_000_000}

type Result struct {
	batchSize  int
	duration   time.Duration
	rowsPerSec float64
}

func main() {
	ctx := context.Background()

	// Load .env file if it exists (not fatal if missing)
	_ = godotenv.Load()

	// Get database connection string from environment
	connString := os.Getenv("DATABASE_URL")
	if connString == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Connect to database
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Run migrations
	if err := runMigrations(connString); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	fmt.Println("Generating test data...")
	data := generateData(totalRows)
	fmt.Printf("Generated %d rows\n\n", len(data))

	// Run benchmarks for each batch size
	var results []Result
	for _, batchSize := range batchSizes {
		fmt.Printf("Testing batch size: %d\n", batchSize)

		// Clear table before each test
		if err := clearTable(ctx, pool); err != nil {
			log.Fatalf("Failed to clear table: %v", err)
		}

		duration, err := insertWithBatch(ctx, pool, data, batchSize)
		if err != nil {
			log.Fatalf("Failed to insert data: %v", err)
		}

		rowsPerSec := float64(totalRows) / duration.Seconds()
		results = append(results, Result{
			batchSize:  batchSize,
			duration:   duration,
			rowsPerSec: rowsPerSec,
		})

		fmt.Printf("  Duration: %v\n", duration)
		fmt.Printf("  Throughput: %.0f rows/sec\n\n", rowsPerSec)
	}

	// Display histogram
	displayHistogram(results)
}

func runMigrations(connString string) error {
	goose.SetBaseFS(embedMigrations)

	db, err := goose.OpenDBWithDriver("pgx", connString)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

func generateData(n int) []string {
	data := make([]string, n)
	for i := 0; i < n; i++ {
		data[i] = fmt.Sprintf("test data row %d", i)
	}
	return data
}

func clearTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "TRUNCATE test_data")
	return err
}

func insertWithBatch(ctx context.Context, pool *pgxpool.Pool, data []string, batchSize int) (time.Duration, error) {
	start := time.Now()

	// Process data in transactions of batchSize rows each
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}

		// Create a new transaction for this batch
		tx, err := pool.Begin(ctx)
		if err != nil {
			return 0, err
		}

		// Use pgx.Batch for efficient pipelining within the transaction
		batch := &pgx.Batch{}
		for _, row := range data[i:end] {
			batch.Queue("INSERT INTO test_data (data) VALUES ($1)", row)
		}

		br := tx.SendBatch(ctx, batch)
		if err := br.Close(); err != nil {
			tx.Rollback(ctx)
			return 0, err
		}

		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
	}

	return time.Since(start), nil
}

func displayHistogram(results []Result) {
	fmt.Println("=== Throughput Results ===")
	fmt.Println()

	// Find max throughput for scaling
	maxThroughput := 0.0
	for _, r := range results {
		if r.rowsPerSec > maxThroughput {
			maxThroughput = r.rowsPerSec
		}
	}

	// Display histogram
	const barWidth = 60
	for _, r := range results {
		barLength := int((r.rowsPerSec / maxThroughput) * barWidth)
		bar := ""
		for i := 0; i < barLength; i++ {
			bar += "█"
		}

		fmt.Printf("%-10d | %-60s | %10.0f rows/sec\n", r.batchSize, bar, r.rowsPerSec)
	}
}
