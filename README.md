# postgresql transaction scalability

This projects tries to see how postgresql throughput scales as transaction size increases. It will generate
10M rows and then try to insert them using transaction sizes of 100, 1000, 10000, 100000 and 1M.

The projects uses Goose to manage the embedded migrations. The only configuration needed is the database strong, used by
pgx.

The project tries to draw a simple histogram of the throughput.

## Configuration

The program requires a `DATABASE_URL` environment variable containing the PostgreSQL connection string.

If a `.env` file exists in the current directory, it will be loaded automatically. If the file doesn't exist, the program continues without error. Environment variables already set in the shell take precedence.

Example `.env` file:
```
DATABASE_URL=postgres://user:password@localhost:5432/dbname
```
## Example output

AMD Ryzen 7 9800X3D 8-Core Processor
Ubuntu 24.03.03
PCIe gen 5 NVMe (10+GB per sec)
48GB RAM

```
2025/11/09 08:41:03 goose: no migrations to run. current version: 1
Generating test data...
Generated 10000000 rows

Testing batch size: 100
  Duration: 1m36.256661284s
  Throughput: 103889 rows/sec

Testing batch size: 1000
  Duration: 32.919465608s
  Throughput: 303772 rows/sec

Testing batch size: 10000
  Duration: 26.359998455s
  Throughput: 379363 rows/sec

Testing batch size: 100000
  Duration: 25.542365668s
  Throughput: 391506 rows/sec

Testing batch size: 1000000
  Duration: 26.20720897s
  Throughput: 381574 rows/sec

=== Throughput Results ===

100        | ███████████████                                              |     103889 rows/sec
1000       | ██████████████████████████████████████████████               |     303772 rows/sec
10000      | ██████████████████████████████████████████████████████████   |     379363 rows/sec
100000     | ████████████████████████████████████████████████████████████ |     391506 rows/sec
1000000    | ██████████████████████████████████████████████████████████   |     381574 rows/sec
```


## Dependencies

We try to keep the dependencies to a minimum, mostly using stdlib and the pgx driver. This includes any tests written.


