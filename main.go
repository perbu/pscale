package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"math"
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

var batchSizes = []int{100, 1000, 10_000, 100_000, 1_000_000, 10_000_000}

type TestRow struct {
	data        string
	description string
	counter1    int
	counter2    int
}

type Result struct {
	batchSize  int
	duration   time.Duration
	rowsPerSec float64
	stdDev     float64
	samples    int
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

		// Run warmup transactions
		if err := runWarmup(ctx, pool, data, batchSize); err != nil {
			log.Fatalf("Failed to run warmup: %v", err)
		}

		// Measure steady-state performance
		result, err := measureSteadyState(ctx, pool, data, batchSize)
		if err != nil {
			log.Fatalf("Failed to measure steady state: %v", err)
		}

		results = append(results, result)

		fmt.Printf("  Throughput: %.0f ± %.0f rows/sec (%d samples)\n\n",
			result.rowsPerSec, result.stdDev, result.samples)
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

func generateData(n int) []TestRow {
	data := make([]TestRow, n)
	for i := 0; i < n; i++ {
		data[i] = TestRow{
			data:        fmt.Sprintf("test data row %d", i),
			description: fmt.Sprintf("description for row %d with some additional text to make it more realistic", i),
			counter1:    i * 2,
			counter2:    i * 3,
		}
	}
	return data
}

func clearTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "TRUNCATE test_data")
	return err
}

func insertWithBatch(ctx context.Context, pool *pgxpool.Pool, data []TestRow, batchSize int) (time.Duration, error) {
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
			batch.Queue("INSERT INTO test_data (data, description, counter1, counter2) VALUES ($1, $2, $3, $4)",
				row.data, row.description, row.counter1, row.counter2)
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

// runWarmup runs warmup transactions to ensure database is in steady state
func runWarmup(ctx context.Context, pool *pgxpool.Pool, data []TestRow, batchSize int) error {
	fmt.Println("  Running warmup transactions...")
	for i := 0; i < 2; i++ {
		// Use a small subset of data for warmup
		warmupSize := batchSize
		if warmupSize > len(data) {
			warmupSize = len(data)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		batch := &pgx.Batch{}
		for j := 0; j < warmupSize; j++ {
			row := data[j]
			batch.Queue("INSERT INTO test_data (data, description, counter1, counter2) VALUES ($1, $2, $3, $4)",
				row.data, row.description, row.counter1, row.counter2)
		}

		br := tx.SendBatch(ctx, batch)
		if err := br.Close(); err != nil {
			tx.Rollback(ctx)
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	// Clear the warmup data
	if err := clearTable(ctx, pool); err != nil {
		return err
	}

	return nil
}

// measureSteadyState runs the benchmark until performance stabilizes
func measureSteadyState(ctx context.Context, pool *pgxpool.Pool, data []TestRow, batchSize int) (Result, error) {
	const (
		minSamples       = 5      // Minimum number of samples before checking stability
		maxSamples       = 20     // Maximum samples to prevent infinite loops
		targetCV         = 0.05   // Target coefficient of variation (5%)
		sampleSize       = 100_000 // Number of rows per sample
	)

	var durations []float64
	var totalRows int

	for len(durations) < maxSamples {
		// Clear table before each sample
		if err := clearTable(ctx, pool); err != nil {
			return Result{}, err
		}

		// Determine how many rows to insert for this sample
		rowsToInsert := sampleSize
		if rowsToInsert > len(data) {
			rowsToInsert = len(data)
		}

		// Measure this sample
		duration, err := insertWithBatch(ctx, pool, data[:rowsToInsert], batchSize)
		if err != nil {
			return Result{}, err
		}

		rowsPerSec := float64(rowsToInsert) / duration.Seconds()
		durations = append(durations, rowsPerSec)
		totalRows += rowsToInsert

		// Check if we've reached steady state
		if len(durations) >= minSamples {
			mean := calculateMean(durations)
			stdDev := calculateStdDev(durations, mean)
			cv := stdDev / mean

			fmt.Printf("    Sample %d: %.0f rows/sec (mean: %.0f, CV: %.2f%%)\n",
				len(durations), rowsPerSec, mean, cv*100)

			if cv <= targetCV {
				fmt.Printf("  Reached steady state after %d samples (CV: %.2f%%)\n", len(durations), cv*100)
				return Result{
					batchSize:  batchSize,
					duration:   time.Duration(float64(time.Second) * float64(totalRows) / mean),
					rowsPerSec: mean,
					stdDev:     stdDev,
					samples:    len(durations),
				}, nil
			}
		} else {
			fmt.Printf("    Sample %d: %.0f rows/sec\n", len(durations), rowsPerSec)
		}
	}

	// Reached max samples without stabilizing
	mean := calculateMean(durations)
	stdDev := calculateStdDev(durations, mean)
	cv := stdDev / mean
	fmt.Printf("  Reached max samples (%d) with CV: %.2f%%\n", maxSamples, cv*100)

	return Result{
		batchSize:  batchSize,
		duration:   time.Duration(float64(time.Second) * float64(totalRows) / mean),
		rowsPerSec: mean,
		stdDev:     stdDev,
		samples:    len(durations),
	}, nil
}

func calculateMean(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDev(values []float64, mean float64) float64 {
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	variance := sumSquares / float64(len(values))
	return math.Sqrt(variance)
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
	const barWidth = 50
	for _, r := range results {
		barLength := int((r.rowsPerSec / maxThroughput) * barWidth)
		bar := ""
		for i := 0; i < barLength; i++ {
			bar += "█"
		}

		cv := (r.stdDev / r.rowsPerSec) * 100
		fmt.Printf("%-11d | %-50s | %10.0f ± %6.0f rows/sec (CV: %4.1f%%, n=%d)\n",
			r.batchSize, bar, r.rowsPerSec, r.stdDev, cv, r.samples)
	}
}
