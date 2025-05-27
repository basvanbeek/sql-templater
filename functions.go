package sqltemplater

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"text/template"
)

var (
	errReturnTypeMismatch = errors.New("function must return (string) or (string, error)")
	FuncMap               = newFuncMap()
)

type TFuncMap interface {
	GetFuncMap() template.FuncMap
}

var _ TFuncMap = (*funcMap)(nil)

type funcMap struct {
	fm template.FuncMap
}

func newFuncMap() *funcMap {
	fm := make(template.FuncMap)

	// Add our default functions
	fm["in"] = in
	fm["positionalParam"] = positionalParam

	return &funcMap{fm: fm}
}

func (f *funcMap) GetFuncMap() template.FuncMap {
	return f.fm
}

func (f *funcMap) AddFunc(name string, fn interface{}) error {
	if _, exists := f.fm[name]; exists {
		return errors.New("function name already registered: " + name)
	}

	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return errors.New("provided value is not a function")
	}

	// Check the number of return values
	switch fnType.NumOut() {
	case 1:
		if fnType.Out(0).Kind() != reflect.String {
			return errReturnTypeMismatch
		}
	case 2:
		if fnType.Out(0).Kind() != reflect.String || fnType.Out(1).Name() != "error" {
			return errReturnTypeMismatch
		}
	default:
		return errReturnTypeMismatch
	}

	f.fm[name] = fn

	return nil
}

func in(values interface{}) (string, error) {
	var count int
	//nolint:exhaustive // we only need to test for two types of types ;)
	switch reflect.TypeOf(values).Kind() {
	case reflect.Slice:
		count = reflect.ValueOf(values).Len()
	case reflect.Array:
		count = reflect.ValueOf(values).Len()
	default:
		count = 1
	}
	switch count {
	case 0:
		return "", errors.New("we must have at least one item for a like clause value")
	case 1:
		return " = ?", nil
	default:
		return " IN (" + strings.TrimSuffix(strings.Repeat("?,", count), ",") + ")", nil
	}
}

func positionalParam(counter *int) string {
	if counter == nil {
		return "?"
	}
	*counter++
	return "$" + strconv.Itoa(*counter)
}
