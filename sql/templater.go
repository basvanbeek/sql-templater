package sql

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/basvanbeek/telemetry/scope"
	"google.golang.org/protobuf/proto"

	sql_templater "github.com/basvanbeek/sql-templater"
)

var log = scope.Register("sql-templater", "messages from the SQL templating logic")

type ParamHandler interface {
	proto.Message
	Params() []interface{}
}

type TemplatedQuery struct {
	tpl *template.Template
	db  *sql.DB
	obj ParamHandler
}

func InitTemplate(name, tpl string) (*template.Template, error) {
	if sql_templater.FuncMap == nil {
		return nil, errors.New("function map is not initialized")
	}

	return template.New(name).Funcs(sql_templater.FuncMap.GetFuncMap()).Parse(tpl)
}

func NewTemplatedQuery(
	tpl *template.Template, db *sql.DB, obj ParamHandler,
) *TemplatedQuery {
	return &TemplatedQuery{tpl: tpl, db: db, obj: obj}
}

func (t *TemplatedQuery) GetQuery(
	ctx context.Context, sortOrder string,
) (rows *sql.Rows, err error) {
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
		"params", fmt.Sprintf("%+v", t.obj.Params()))
	rows, err = t.db.QueryContext(ctx, query, t.obj.Params()...)
	l.Debug("get query done",
		"duration", time.Since(startTime).String())
	return
}

func (t *TemplatedQuery) GetCount(ctx context.Context) (count int, err error) {
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
		"params", fmt.Sprintf("%+v", t.obj.Params()))
	err = t.db.QueryRowContext(ctx, query, t.obj.Params()...).Scan(&count)
	l.Debug("count query done",
		"count", count,
		"duration", time.Since(startTime).String())
	return
}

func (t *TemplatedQuery) GetPage(
	ctx context.Context, limit, offset int, sortOrder string,
) (rows *sql.Rows, err error) {
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
		"params", fmt.Sprintf("%+v", t.obj.Params()))
	rows, err = t.db.QueryContext(ctx, query, t.obj.Params()...)
	l.Debug("page query done",
		"duration", time.Since(startTime).String())
	return
}
