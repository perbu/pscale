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

## Dependencies

We try to keep the dependencies to a minimum, mostly using stdlib and the pgx driver. This includes any tests written.


