# enrollments

## Running the app
To run the docker-compose stack (which has the app and postgres container), make sure that docker is installed on your machine, and then run the following command while on the root of the project directory.

```
docker compose down -v
docker compose up --build
```

Once the app is running,

```
enrollments-db-1   | 2026-11-03 17:35:28.919 UTC [1] LOG:  listening on Unix socket "/var/run/postgresql/.s.PGSQL.5432"
enrollments-db-1   | 2026-11-03 17:35:28.924 UTC [57] LOG:  database system was shut down at 2026-11-03 17:35:28 UTC
enrollments-db-1   | 2026-11-03 17:35:28.928 UTC [1] LOG:  database system is ready to accept connections
enrollments-app-1  | 2026/11/03 17:35:30 Work by Joshua Nario.  See ASSIGNMENT.MD
enrollments-app-1  | 2026/11/03 17:35:30 connected to postgres, schema migrated
enrollments-app-1  | 2026/11/03 17:35:30 seed: upserted path web-basics [L1 L2 L3]
enrollments-app-1  | 2026/11/03 17:35:30 seed: upserted path secure-go [G1 G2 G3]
enrollments-app-1  | 2026/11/03 17:35:30 seed: upserted path legacy-path [X1 X2]
enrollments-app-1  | 2026/11/03 17:35:30 enrollments is running on http://localhost:8080
```

open a browser to the url `http://localhost:8080`.

Select between profiles (like alice and bob) on the top right and hit the play button to go through each scenario.  Note that the `setup` playlist is redundant and make no meaningful change as the postgres is pre-seeded with the paths.  While the scenario is playing, the viewer has live updates capability so you can watch as the events and actions stream in to the server.

To stop the docker-compose stack, send an interrupt signal to the console where docker-compose is running with a Ctrl+C (Windows) or Cmd+C (Mac).

## Running the test cases

To run all the test cases, run the following command while on the root of the project directory.

```
go test ./...
```