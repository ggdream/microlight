//go:build js && wasm

package sqliteweb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"syscall/js"
	"time"
)

const DriverName = "sqliteweb"

func init() { sql.Register(DriverName, &sqliteDriver{}) }

// Open opens an origin-local, persistent database. Name must contain 1–48 ASCII
// letters, digits, underscores or hyphens. Serve the web/ assets at ./sqlite/.
func Open(name string) (*sql.DB, error) {
	db, err := sql.Open(DriverName, name)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Error preserves SQLite's numeric result code across the worker boundary.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("browser sqlite (%d): %s", e.Code, e.Message) }

var loadOnce sync.Once
var loadErr error
var api js.Value

func load() error {
	loadOnce.Do(func() {
		document := js.Global().Get("document")
		if !document.Truthy() {
			loadErr = errors.New("browser SQLite requires a document")
			return
		}
		script := document.Call("createElement", "script")
		script.Set("type", "module")
		script.Set("src", js.Global().Get("URL").New("sqlite/bridge.js", document.Get("baseURI")).Get("href"))
		done := make(chan error, 1)
		onLoad := js.FuncOf(func(js.Value, []js.Value) any { done <- nil; return nil })
		onError := js.FuncOf(func(js.Value, []js.Value) any { done <- errors.New("failed to load browser SQLite assets"); return nil })
		script.Set("onload", onLoad)
		script.Set("onerror", onError)
		document.Get("head").Call("appendChild", script)
		loadErr = <-done
		script.Set("onload", js.Null())
		script.Set("onerror", js.Null())
		onLoad.Release()
		onError.Release()
		if loadErr == nil {
			api = js.Global().Get("microlightSQLite")
		}
	})
	return loadErr
}

func call(ctx context.Context, op string, args ...any) (js.Value, error) {
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	if err := load(); err != nil {
		return js.Undefined(), err
	}
	if err := ctx.Err(); err != nil {
		return js.Undefined(), err
	}
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any { done <- result{value: args[0]}; return nil })
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v := args[0]
		code := 0
		if n := v.Get("code"); n.Type() == js.TypeNumber {
			code = n.Int()
		}
		done <- result{err: &Error{Code: code, Message: v.Get("message").String()}}
		return nil
	})
	api.Call("call", op, js.ValueOf(args)).Call("then", ok, bad)
	// Always drain an operation already submitted. Returning early on cancellation
	// could allow database/sql to reuse a connection while its worker still writes.
	res := <-done
	ok.Release()
	bad.Release()
	return res.value, res.err
}

type sqliteDriver struct{}

func (*sqliteDriver) Open(name string) (driver.Conn, error) {
	v, err := call(context.Background(), "open", name)
	if err != nil {
		return nil, err
	}
	return &conn{id: v.Int()}, nil
}

type conn struct {
	id     int
	closed bool
}

func (c *conn) Close() error {
	if c.closed {
		return nil
	}
	_, err := call(context.Background(), "close", c.id)
	c.closed = true
	return err
}
func (c *conn) Ping(ctx context.Context) error {
	_, err := c.ExecContext(ctx, "SELECT 1", nil)
	return err
}
func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	v, err := call(ctx, "prepare", c.id, query)
	if err != nil {
		return nil, err
	}
	s := &stmt{conn: c, id: v.Get("id").Int(), count: v.Get("count").Int(), query: query}
	columns := v.Get("columns")
	for i := 0; i < columns.Length(); i++ {
		s.columns = append(s.columns, columns.Index(i).String())
	}
	return s, nil
}
func (c *conn) Begin() (driver.Tx, error) { return c.BeginTx(context.Background(), driver.TxOptions{}) }
func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly || opts.Isolation != driver.IsolationLevel(sql.LevelDefault) {
		return nil, errors.New("browser SQLite does not support these transaction options")
	}
	_, err := c.ExecContext(ctx, "BEGIN", nil)
	if err != nil {
		return nil, err
	}
	return &transaction{conn: c}, nil
}
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	parameters, err := encode(args)
	if err != nil {
		return nil, err
	}
	v, err := call(ctx, "exec", c.id, query, parameters)
	if err != nil {
		return nil, err
	}
	inserted, err := strconv.ParseInt(v.Get("inserted").String(), 10, 64)
	if err != nil {
		return nil, err
	}
	return result{inserted: inserted, affected: int64(v.Get("affected").Float())}, nil
}
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	prepared, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	s := prepared.(*stmt)
	r, err := s.QueryContext(ctx, args)
	if err != nil {
		s.Close()
		return nil, err
	}
	r.(*rows).ownStatement = true
	return r, nil
}

func encode(args []driver.NamedValue) ([]any, error) {
	values := make([]any, len(args))
	for i, arg := range args {
		if arg.Name != "" {
			return nil, errors.New("browser SQLite requires positional parameters")
		}
		switch v := arg.Value.(type) {
		case nil:
			values[i] = []any{"null", nil}
		case int64:
			values[i] = []any{"integer", strconv.FormatInt(v, 10)}
		case float64:
			values[i] = []any{"real", v}
		case bool:
			n := "0"
			if v {
				n = "1"
			}
			values[i] = []any{"integer", n}
		case string:
			values[i] = []any{"text", v}
		case []byte:
			if v == nil {
				values[i] = []any{"null", nil}
				continue
			}
			data := js.Global().Get("Uint8Array").New(len(v))
			js.CopyBytesToJS(data, v)
			values[i] = []any{"blob", data}
		case time.Time:
			values[i] = []any{"text", v.Format("2006-01-02 15:04:05.999999999-07:00")}
		default:
			return nil, fmt.Errorf("unsupported browser SQLite parameter %T", v)
		}
	}
	return values, nil
}

type result struct{ inserted, affected int64 }

func (r result) LastInsertId() (int64, error) { return r.inserted, nil }
func (r result) RowsAffected() (int64, error) { return r.affected, nil }

type transaction struct{ conn *conn }

func (t *transaction) Commit() error {
	_, err := t.conn.ExecContext(context.Background(), "COMMIT", nil)
	if err != nil {
		// database/sql considers this Tx finished even when COMMIT fails. Do not
		// return a connection with an active transaction to its pool.
		_, rollbackErr := t.conn.ExecContext(context.Background(), "ROLLBACK", nil)
		if rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
	}
	return err
}
func (t *transaction) Rollback() error {
	_, err := t.conn.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

type stmt struct {
	conn      *conn
	id, count int
	query     string
	columns   []string
	closed    bool
}

func (s *stmt) NumInput() int { return s.count }
func (s *stmt) Close() error {
	if s.closed {
		return nil
	}
	_, err := call(context.Background(), "finalize", s.conn.id, s.id)
	s.closed = true
	return err
}
func named(values []driver.Value) []driver.NamedValue {
	args := make([]driver.NamedValue, len(values))
	for i, v := range values {
		args[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return args
}
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), named(args))
}
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), named(args))
}
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	parameters, err := encode(args)
	if err != nil {
		return nil, err
	}
	if _, err := call(ctx, "query", s.conn.id, s.id, parameters); err != nil {
		return nil, err
	}
	return &rows{stmt: s, ctx: ctx}, nil
}

type rows struct {
	stmt                 *stmt
	ctx                  context.Context
	ownStatement, closed bool
}

func (r *rows) Columns() []string { return r.stmt.columns }
func (r *rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.ownStatement {
		return r.stmt.Close()
	}
	_, err := call(context.Background(), "reset", r.stmt.conn.id, r.stmt.id)
	return err
}
func (r *rows) Next(dest []driver.Value) error {
	v, err := call(r.ctx, "next", r.stmt.conn.id, r.stmt.id)
	if err != nil {
		return err
	}
	if v.IsNull() {
		return io.EOF
	}
	for i := range dest {
		cell := v.Index(i)
		data := cell.Index(1)
		switch cell.Index(0).Int() {
		case 1:
			dest[i], err = strconv.ParseInt(data.String(), 10, 64)
		case 2:
			dest[i] = data.Float()
		case 3:
			dest[i] = data.String()
		case 4:
			b := make([]byte, data.Get("byteLength").Int())
			js.CopyBytesToGo(b, data)
			dest[i] = b
		case 5:
			dest[i] = nil
		default:
			return errors.New("unknown SQLite result type")
		}
		if err != nil {
			return err
		}
	}
	return nil
}
