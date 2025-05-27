package pgx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/basvanbeek/sql-templater"
	"github.com/basvanbeek/telemetry/scope"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

var log = scope.Register("sql-templater", "messages from the SQL templating logic")

type ParamHandler interface {
	proto.Message
	Params() []interface{}
}

type option struct {
	name        string
	oldSchema   string
	schemaParam string
	funcMap     template.FuncMap
}

type Option func(o *option) error

type TemplatedQuery struct {
	tpl *template.Template
	db  *pgxpool.Pool
	obj ParamHandler
}

func WithName(name string) Option {
	return func(o *option) error {
		if name == "" {
			return errors.New("name cannot be empty")
		}
		o.name = name
		return nil
	}
}

func WithDynamicSchema(schemaPlaceholder, schemaParam string) Option {
	return func(o *option) error {
		if schemaPlaceholder == "" || schemaParam == "" {
			return errors.New("schema placeholder and parameter cannot be empty")
		}
		o.oldSchema = schemaPlaceholder
		o.schemaParam = schemaParam
		return nil
	}
}

func WithFuncMap(funcMap template.FuncMap) Option {
	return func(o *option) error {
		if funcMap == nil {
			return errors.New("function map cannot be nil")
		}
		o.funcMap = funcMap
		return nil
	}
}

func InitTemplate(tpl string, opts ...Option) (*template.Template, error) {
	o := &option{
		name:        "tpl",
		oldSchema:   "",
		schemaParam: "",
		funcMap:     sqltemplater.FuncMap.GetFuncMap(),
	}
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, fmt.Errorf("error applying option: %w", err)
		}
	}
	if sqltemplater.FuncMap == nil {
		return nil, errors.New("function map is not initialized")
	}

	if o.oldSchema != "" {
		tpl = strings.ReplaceAll(tpl, `"`+o.oldSchema+`"`, `"{{ .`+o.schemaParam+` }}"`)
	}

	return template.New(o.name).Funcs(o.funcMap).Parse(tpl)
}

func NewTemplatedQuery(
	tpl *template.Template, db *pgxpool.Pool, obj ParamHandler,
) *TemplatedQuery {
	return &TemplatedQuery{tpl: tpl, db: db, obj: obj}
}

func (t *TemplatedQuery) Execute(ctx context.Context) (rows pgx.Rows, err error) {
	params := t.obj.Params()
	var buf bytes.Buffer
	if err = t.tpl.Execute(&buf, t.obj); err != nil {
		err = fmt.Errorf(
			"unable to execute template %s with object %+v: %w",
			t.tpl.Name(), t.obj, err)
		log.Context(ctx).Error("Execute failed", err,
			"object", t.tpl.DefinedTemplates())
		return nil, err
	}
	query := buf.String()
	startTime := time.Now()
	log.Debug("query details",
		"query", query,
		"params", fmt.Sprintf("%+v", params))
	rows, err = t.db.Query(ctx, query, params...)
	log.Debug("get query done",
		"duration", time.Since(startTime).String())
	return
}

func (t *TemplatedQuery) GetQuery(
	ctx context.Context, sortOrder string,
) (rows pgx.Rows, err error) {
	params := t.obj.Params()
	var buf bytes.Buffer
	l := log.Context(ctx).With("sort_order", sortOrder)
	if err = t.tpl.Execute(&buf, t.obj); err != nil {
		err = fmt.Errorf(
			"unable to execute template %s with object %+v: %w",
			t.tpl.Name(), t.obj, err)
		l.Error("GetQuery failed", err,
			"object", t.tpl.DefinedTemplates())
		return nil, err
	}
	query := buf.String()
	if sortOrder != "" {
		query += " ORDER BY " + sortOrder
	}
	startTime := time.Now()
	l.Debug("query details",
		"query", query,
		"params", fmt.Sprintf("%+v", params))
	rows, err = t.db.Query(ctx, query, params...)
	l.Debug("get query done",
		"duration", time.Since(startTime).String())
	return
}

func (t *TemplatedQuery) GetCount(ctx context.Context) (count int, err error) {
	params := t.obj.Params()
	var buf bytes.Buffer
	l := log.Context(ctx)
	if err = t.tpl.Execute(&buf, t.obj); err != nil {
		err = fmt.Errorf(
			"unable to execute template %s with object %+v: %w",
			t.tpl.Name(), t.obj, err)
		l.Error("GetCount failed", err,
			"object", t.tpl.DefinedTemplates())
		return 0, err
	}
	//nolint:gosec // explicit sql string injection. buf is a trusted source
	query := fmt.Sprintf("SELECT count(*) FROM ( %s ) count_query;", buf.String())
	startTime := time.Now()
	l.Debug("query details",
		"query", query,
		"params", fmt.Sprintf("%+v", params))
	err = t.db.QueryRow(ctx, query, params...).Scan(&count)
	l.Debug("count query done",
		"count", count,
		"duration", time.Since(startTime).String())
	return
}

func (t *TemplatedQuery) GetPage(
	ctx context.Context, limit, offset int, sortOrder string,
) (rows pgx.Rows, err error) {
	params := t.obj.Params()
	var buf bytes.Buffer
	l := log.Context(ctx).With(
		"limit", limit, "offset", offset, "sort_order", sortOrder,
	)
	if err = t.tpl.Execute(&buf, t.obj); err != nil {
		err = fmt.Errorf(
			"unable to execute template %s with object %+v: %w",
			t.tpl.Name(), t.obj, err)
		l.Error("GetPage failed", err,
			"object", t.tpl.DefinedTemplates())
		return nil, err
	}
	var query string
	if limit == 0 && offset == 0 {
		query = fmt.Sprintf("%s ORDER BY %s", buf.String(), sortOrder)
	} else {
		query = fmt.Sprintf(
			"%s ORDER BY %s LIMIT %d OFFSET %d",
			buf.String(), sortOrder, limit, offset)
	}
	startTime := time.Now()
	l.Debug("query details",
		"query", query,
		"params", fmt.Sprintf("%+v", params))
	rows, err = t.db.Query(ctx, query, params...)
	l.Debug("page query done",
		"duration", time.Since(startTime).String())
	return
}
