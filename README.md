# gothic TodoMVC

A standalone copy of the gothic TodoMVC example. It lives outside the core gothic
repository and imports gothic as a normal Go module.

## Run

```sh
go generate
go run .
```

Open <http://localhost:8080>. Set `PORT` to change the port.

The app uses an in-memory SQLite database by default. Set `DB_PATH=/path/to/todo.db`
to use a file.

## Verify

```sh
go test ./...
```

## Dependency

The app pins a gothic pseudo-version from the current core `main` branch because
the latest features are not tagged yet. When the next core release is tagged,
update the requirement to that release.

## What it shows

- TodoMVC behavior: add, toggle, edit, delete, mark all, clear completed, and filters.
- Commands and procedures with arguments.
- Keyed lists, dynamic attributes, class toggles, focus, and hash routing.
- SQLite persistence with no auth and one shared list.
- A gothic app built entirely outside the gothic repository.
