//go:generate go run github.com/metruzanca/gothic/cmd/gothic generate .

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
	_ "modernc.org/sqlite"
)

//go:embed style.css
var styleCSS []byte

type store struct {
	db *sql.DB
}

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

func (s *store) list(ctx context.Context) (TodoData, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, completed FROM todos ORDER BY id`)
	if err != nil {
		return TodoData{}, err
	}
	defer rows.Close()

	todos := make([]Todo, 0)
	active := 0
	completed := 0
	for rows.Next() {
		var todo Todo
		var done int
		if err := rows.Scan(&todo.ID, &todo.Title, &done); err != nil {
			return TodoData{}, err
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
		return TodoData{}, err
	}

	word := "items"
	if active == 1 {
		word = "item"
	}
	return TodoData{
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

type procedures struct {
	store *store
}

func (p procedures) ListTodos(ctx context.Context) (TodoData, error) {
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
	RegisterProcedures(s, procedures{store: st})
	mountApp(s, st)
	log.Fatal(s.ListenAndServe(addr))
}

func mountApp(s *gothic.Server, st *store) {
	s.Get("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(styleCSS)
	})
	s.Get("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := st.list(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var page bytes.Buffer
		page.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Gothic TodoMVC</title><link rel="stylesheet" href="/style.css"></head><body>`)
		if err := gothic.NewApp(NewModel(data)).RenderTo(r.Context(), &page); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page.WriteString(`</body></html>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page.Bytes())
	})
}
