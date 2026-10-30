# gothic TodoMVC

A standalone copy of the gothic TodoMVC example. It lives outside the core gothic
repository and imports gothic as a normal Go module.

## Live demo

<https://gothic-todomvc-production.up.railway.app>

## Structure

```
cmd/todomvc/   entrypoint and HTTP server
src/           .gth app and component sources, plus the generator directive
Makefile       generate, run, test, and build shortcuts
```

Generated Go and JavaScript files are not committed. Run the generator before
building or testing:

```sh
go generate ./src
```

## How it fits together

gothic follows The Elm Architecture, the same shape as Bubble Tea:

```text
DOM event -> Msg -> Update -> (new Model, optional Cmd) -> render
                               ^                          |
                               +---------- Msg <----------+
```

- `Model` is the whole client state. It is a plain value, not a mutable store.
- `Msg` is an event value such as `AddClicked` or `TodosLoaded`.
- `Update` returns the next model and an optional command.
- `Cmd` performs async work, usually a procedure call, and returns a `Msg`.
- `view` renders the model. Go uses it for the first paint, and the generator
  compiles it to JavaScript for the browser.
- Procedures are Go methods on the server, declared as an interface in the
  app file. The generator creates client stubs and the server registration.

The loop is: an event dispatches a message, `Update` produces a new model, the
generated render function applies that model to the DOM, and any command runs
afterwards and sends its result back as another message.

### Coming from React

| React | gothic |
| --- | --- |
| `useState` and `setState` | fields on `Model`, changed in `Update` |
| event handler closure | a `Msg` value passed to `onclick` and similar attributes |
| component and props | `view` function and its parameters, inlined at compile time |
| `useEffect` | `Cmd` |
| virtual DOM diff | compiled bindings that patch existing DOM nodes |
| context or prop drilling | plain fields on the shared `Model` |

The biggest difference is event handling. React takes a function:

```jsx
<button onClick={() => setCount(count + 1)}>+</button>
```

gothic takes a message value:

```gothic
<button onclick={ IncrementClicked{} }>+</button>
```

The generator wires the listener to dispatch that message. When the event needs
data, build it into the message, for example `ToggleClicked{ID: todo.ID}`. Use
`event.target` only for values that exist solely at event time, such as the text
in an input.

### Rendering

The first request is rendered in Go, so the page works before JavaScript loads.
The generated HTML includes the initial model as JSON and a module script. The
client runtime adopts that DOM, binds handlers, and from then on runs `Update`
and `render` for each message.

Rendering has no virtual DOM. The compiler marks each dynamic text node,
attribute, class, and list, so `render` writes only to the parts of the DOM that
depend on the model. Elements are reused, and keyed list items keep their
identity across reorders and filter changes.

### Where to look

1. `src/app.gth`: the model, messages, procedures, update loop, and root view.
2. `src/todo_item.gth`: a stateless component and keyed list usage.
3. `cmd/todomvc/main.go`: persistence, procedure implementations, and SSR.

## Run

```sh
make run
```

Or:

```sh
go generate ./src
go run ./cmd/todomvc
```

Open <http://localhost:8080>. Set `PORT` to change the port.

The app uses an in-memory SQLite database by default. Set
`DB_PATH=/path/to/todo.db` to use a file.

## Verify

```sh
make test
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
