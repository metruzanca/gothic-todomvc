package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/metruzanca/gothic"
	app "github.com/metruzanca/gothic-todomvc/src"
	_ "modernc.org/sqlite"
)

//go:embed style.css
var styleCSS []byte

// This file is the server side of the app. It has three jobs:
//
//  1. Persist todos in SQLite.
//  2. Implement the procedures declared in src/app.gth.
//  3. Serve the first HTML paint, including the initial model.
//
// The UI code lives in src/app.gth, and its comments start the tour. The
// wiring here assumes that file.

// store is a thin SQLite wrapper. There is no auth and one shared list, which
// keeps the example focused on gothic instead of tenancy.
type store struct {
	db *sql.DB
}

// openStore opens the database and creates the table on first run. The app uses
// an in-memory database unless DB_PATH is set.
func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS todos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) close() error {
	return s.db.Close()
}

// list reads the full list and computes the derived fields the view needs. The
// server owns these counts so the browser never recomputes them after an edit,
// a filter change, or a reload.
func (s *store) list(ctx context.Context) (app.TodoData, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, completed FROM todos ORDER BY id`)
	if err != nil {
		return app.TodoData{}, err
	}
	defer rows.Close()

	todos := make([]app.Todo, 0)
	active := 0
	completed := 0
	for rows.Next() {
		var todo app.Todo
		var done int
		if err := rows.Scan(&todo.ID, &todo.Title, &done); err != nil {
			return app.TodoData{}, err
		}
		todo.Completed = done != 0
		if todo.Completed {
			completed++
		} else {
			active++
		}
		todos = append(todos, todo)
	}
	if err := rows.Err(); err != nil {
		return app.TodoData{}, err
	}

	word := "items"
	if active == 1 {
		word = "item"
	}
	return app.TodoData{
		Todos:          todos,
		ActiveCount:    active,
		CompletedCount: completed,
		HasTodos:       len(todos) > 0,
		HasCompleted:   completed > 0,
		AllDone:        len(todos) > 0 && active == 0,
		ItemWord:       word,
	}, nil
}

func (s *store) add(ctx context.Context, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("todo title is empty")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO todos(title, completed) VALUES(?, 0)`, title)
	return err
}

// setTitle deletes the row when the new title is blank, matching TodoMVC.
func (s *store) setTitle(ctx context.Context, id int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return s.delete(ctx, id)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE todos SET title = ? WHERE id = ?`, title, id)
	return err
}

func (s *store) toggle(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE todos SET completed = CASE completed WHEN 0 THEN 1 ELSE 0 END WHERE id = ?`, id)
	return err
}

func (s *store) setAllCompleted(ctx context.Context, completed bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE todos SET completed = ?`, completed)
	return err
}

func (s *store) delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM todos WHERE id = ?`, id)
	return err
}

func (s *store) clearCompleted(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM todos WHERE completed = 1`)
	return err
}

// procedures implements the Procedures interface declared in src/app.gth. The
// generator produced RegisterProcedures, which adapts these methods to JSON
// RPC: arguments are decoded, context comes from the request, and a returned
// error becomes an RPC error.
//
// In the browser, an error from one of these methods ends up as TodoFailed,
// which Update stores in model.Failure. Procedures are never called directly
// from Update; they are always called inside a Cmd.
type procedures struct {
	store *store
}

func (p procedures) ListTodos(ctx context.Context) (app.TodoData, error) {
	return p.store.list(ctx)
}

func (p procedures) AddTodo(ctx context.Context, title string) error {
	return p.store.add(ctx, title)
}

func (p procedures) SetTodoTitle(ctx context.Context, id int64, title string) error {
	return p.store.setTitle(ctx, id, title)
}

func (p procedures) ToggleTodo(ctx context.Context, id int64) error {
	return p.store.toggle(ctx, id)
}

func (p procedures) SetAllCompleted(ctx context.Context, completed bool) error {
	return p.store.setAllCompleted(ctx, completed)
}

func (p procedures) DeleteTodo(ctx context.Context, id int64) error {
	return p.store.delete(ctx, id)
}

func (p procedures) ClearCompleted(ctx context.Context) error {
	return p.store.clearCompleted(ctx)
}

func main() {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = ":memory:"
	}
	st, err := openStore(path)
	if err != nil {
		log.Fatal(err)
	}
	defer st.close()

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	s := gothic.NewServer()
	// RegisterProcedures mounts one POST route per procedure under
	// /_gothic/rpc/. This single call connects the interface in src/app.gth to
	// the implementation above.
	app.RegisterProcedures(s, procedures{store: st})
	mountApp(s, st)
	log.Fatal(s.ListenAndServe(addr))
}

// mountApp serves the page. It does not use Server.MountApp because the initial
// model depends on the database: the page must render with the current todos so
// the first paint is useful before JavaScript runs.
//
// The generated App view writes the markup and appends two things:
//
//   - a script tag containing the initial model as JSON
//   - a module script that loads the generated client code
//
// The client runtime reads that JSON, adopts the server-rendered DOM instead of
// recreating it, binds the event handlers, and starts the Update/render loop.
func mountApp(s *gothic.Server, st *store) {
	s.Get("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(styleCSS)
	})
	s.Get("/", func(w http.ResponseWriter, r *http.Request) {
		// Build the initial model from server truth, then render the full page.
		data, err := st.list(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var page bytes.Buffer
		page.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Gothic TodoMVC</title><link rel="stylesheet" href="/style.css"></head><body>`)
		if err := gothic.NewApp(app.NewModel(data)).RenderTo(r.Context(), &page); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page.WriteString(`</body></html>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page.Bytes())
	})
}
