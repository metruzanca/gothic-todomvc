package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/metruzanca/gothic"
)

func TestTodoApp(t *testing.T) {
	ctx := context.Background()
	st, err := openStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.close() })

	if err := st.add(ctx, "write tests"); err != nil {
		t.Fatal(err)
	}
	if err := st.add(ctx, "ship it"); err != nil {
		t.Fatal(err)
	}

	s := gothic.NewServer()
	RegisterProcedures(s, procedures{store: st})
	mountApp(s, st)
	ts := httptest.NewServer(s)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	html := string(body)
	for _, want := range []string{
		`<!doctype html>`,
		`class="todoapp"`,
		`"Todos":[{"ID":1,"Title":"write tests","Completed":false},{"ID":2,"Title":"ship it","Completed":false}]`,
		`data-gothic-key="1"`,
		`data-gothic-attr-2="checked"`,
		`data-gothic-class-2="completed"`,
		`data-gothic-focus="0"`,
		`class="new-todo"`,
		`data-gothic-on="0 1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("SSR missing %q\n\n%s", want, html)
		}
	}
	stripped := regexp.MustCompile(`<!--.*?-->`).ReplaceAllString(html, "")
	if !strings.Contains(stripped, "<strong>2</strong> items left") {
		t.Errorf("SSR missing pluralized count\n\n%s", stripped)
	}

	res, err = http.Get(ts.URL + "/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(css), ".todoapp") || !strings.Contains(string(css), ".hidden") {
		t.Fatalf("css response is not TodoMVC CSS")
	}

	res, err = http.Get(ts.URL + "/_gothic/modules/app_gothic.js")
	if err != nil {
		t.Fatal(err)
	}
	js, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, want := range []string{
		`import { attr, classToggle, focus, list, on, readState, rpc, text }`,
		`window.addEventListener("hashchange"`,
		`{tag: "HashChanged", Hash: window.location.hash}`,
		`list(root, "0", (todo, __i0) => {`,
		`(todo, __i0) => String(todo.ID)`,
		`event.key === "Enter"`,
		`event.key === "Escape"`,
		`await AddTodo(title)`,
		`await SetTodoTitle(id, title)`,
		`await ToggleTodo(id)`,
		`await SetAllCompleted(checked)`,
	} {
		if !strings.Contains(string(js), want) {
			t.Errorf("generated JS missing %q\n\n%s", want, js)
		}
	}

	postRPC(t, ts.URL, "AddTodo", `["buy milk"]`)
	data := mustList(t, ts.URL)
	if len(data.Todos) != 3 || data.Todos[2].Title != "buy milk" {
		t.Fatalf("after add: %#v", data)
	}

	postRPC(t, ts.URL, "SetTodoTitle", `[1,"updated title"]`)
	data = mustList(t, ts.URL)
	if data.Todos[0].Title != "updated title" {
		t.Fatalf("after title edit: %#v", data.Todos[0])
	}

	postRPC(t, ts.URL, "ToggleTodo", `[1]`)
	data = mustList(t, ts.URL)
	if !data.Todos[0].Completed || data.ActiveCount != 2 || data.CompletedCount != 1 {
		t.Fatalf("after toggle: %#v", data)
	}

	postRPC(t, ts.URL, "SetAllCompleted", `[true]`)
	data = mustList(t, ts.URL)
	if data.ActiveCount != 0 || !data.AllDone || data.ItemWord != "items" {
		t.Fatalf("after mark all: %#v", data)
	}

	postRPC(t, ts.URL, "ClearCompleted", `[]`)
	data = mustList(t, ts.URL)
	if len(data.Todos) != 0 || data.HasTodos {
		t.Fatalf("after clear completed: %#v", data)
	}

	postRPC(t, ts.URL, "AddTodo", `["shared item"]`)
	res, err = http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "shared item") {
		t.Fatalf("shared list missing added todo\n\n%s", body)
	}
}

type listResponse struct {
	Todos          []Todo `json:"Todos"`
	ActiveCount    int    `json:"ActiveCount"`
	CompletedCount int    `json:"CompletedCount"`
	HasTodos       bool   `json:"HasTodos"`
	HasCompleted   bool   `json:"HasCompleted"`
	AllDone        bool   `json:"AllDone"`
	ItemWord       string `json:"ItemWord"`
}

func mustList(t *testing.T, base string) listResponse {
	t.Helper()
	body := postRPC(t, base, "ListTodos", `[]`)
	var envelope struct {
		Result listResponse `json:"result"`
	}
	if err := jsonUnmarshal(body, &envelope); err != nil {
		t.Fatalf("decode list: %v\n\n%s", err, body)
	}
	return envelope.Result
}

func postRPC(t *testing.T, base, name, args string) []byte {
	t.Helper()
	body := fmt.Sprintf(`{"args":%s}`, args)
	res, err := http.Post(base+"/_gothic/rpc/"+name, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s returned %d: %s", name, res.StatusCode, data)
	}
	return data
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
