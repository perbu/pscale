# postgresql transaction scalability

This projects tries to see how postgresql throughput scales as transaction size increases. It will generate
10M rows and then try to insert them using transaction sizes of 100, 1000, 10000, 100000 and 1M.

The projects uses Goose to manage the embedded migrations. The only configuration needed is the database strong, used by
pgx.

The project tries to draw a simple histogram of the throughput.