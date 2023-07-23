package model_reflect

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/exp/slices"
)

type (
	ModelInfo struct {
		string
		Errs   []error
		Hasher HashInfo
	}
	HashInfo struct {
		Salt    []byte
		Time    uint32
		Memory  uint32
		Threads uint8
	}
)

var (
	DefaultHasher = HashInfo{
		Time:    1,
		Memory:  8,
		Threads: 1,
	}

	DefaultInterfaces = []reflect.Type{
		reflect.TypeOf((*encoding.BinaryMarshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.BinaryUnmarshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem(),
		reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem(),
	}

	ErrLoopDetected = errors.New("loop detected")
	ErrEmptyStruct  = errors.New("empty struct")
	ErrDuplicate    = errors.New("duplicate fields")
)

func New(v any) (m ModelInfo, err error) {
	errs := []error{}
	m = ModelInfo{Hasher: DefaultHasher}
	m.string = typeToString(reflect.TypeOf(v), nil, &errs)
	errs = uniqueErrors(errs)
	if len(errs) > 0 {
		m.Errs = errs
		err = errors.Join(errs...)
	}
	return
}

func (m ModelInfo) Hash() uint64 {
	return binary.LittleEndian.Uint64(
		argon2.IDKey(
			[]byte(m.string),
			m.Hasher.Salt,
			m.Hasher.Time,
			m.Hasher.Memory,
			m.Hasher.Threads,
			8,
		))
}

func (m ModelInfo) String() string {
	return m.string
}

func uniqueErrors(slice []error) []error {
	keys := map[string]bool{}
	list := []error{}
	for _, entry := range slice {
		if _, ok := keys[entry.Error()]; !ok {
			keys[entry.Error()] = true
			list = append(list, entry)
		}
	}
	return list
}

func baseType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func isConcrete(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Func, reflect.Chan:
		return false
	default:
		return true
	}
}

func expandField(f reflect.StructField, types []reflect.Type, result *[][]reflect.StructField) []error {
	errs := []error{}
	depth := len(types)
	for depth >= len(*result) {
		*result = append(*result, []reflect.StructField{})
	}
	t := baseType(f.Type)
	idx := slices.Index(types, t)
	if idx >= 0 {
		errs = append(errs, fmt.Errorf("%w in %s", ErrLoopDetected, t))
		return errs
	}
	types = append(types, t)
	if t.Kind() != reflect.Struct || !f.Anonymous {
		(*result)[depth] = append((*result)[depth], f)
		return nil
	}
	n := t.NumField()
	for i := 0; i < n; i++ {
		errs = append(errs, expandField(t.Field(i), types, result)...)
	}
	return errs
}

func structFields(t reflect.Type) ([]reflect.StructField, []error) {
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	errs := []error{}
	expand := [][]reflect.StructField{}
	n := t.NumField()
	for i := 0; i < n; i++ {
		errs = append(errs, expandField(t.Field(i), nil, &expand)...)
	}
	counts := map[string]int{}
	result := []reflect.StructField{}
	for i, level := range expand {
		localCounts := map[string]int{}
		localIgnore := map[string]bool{}
		for _, f := range level {
			counts[f.Name]++
			localCounts[f.Name]++
		}
		for _, f := range level {
			if f.Tag.Get("reflect") == "-" {
				continue
			}
			if counts[f.Name] == 1 {
				result = append(result, f)
			}
			if !localIgnore[f.Name] && localCounts[f.Name] > 1 {
				errs = append(errs, fmt.Errorf("type %s (embed level %d): %w [%d]%s",
					t, i, ErrDuplicate, localCounts[f.Name], f.Name))
				localIgnore[f.Name] = true
			}
		}
	}
	return result, errs
}

func checkInterfaces(t reflect.Type) []string {
	result := []string{}
	for _, iface := range DefaultInterfaces {
		if reflect.PtrTo(t).Implements(iface) {
			result = append(result, iface.String())
		}
	}
	sort.Strings(result)
	return result
}

func typeToString(t reflect.Type, types []reflect.Type, errs *[]error) string {
	if t != nil {
		t = baseType(t)
	}
	if t == nil || !isConcrete(t) {
		return "?"
	}

	idx := slices.Index(types, t)
	if idx >= 0 {
		*errs = append(*errs, fmt.Errorf("%w in %s", ErrLoopDetected, t))
		return "..."
	}
	types = append(types, t)

	switch t.Kind() {
	case reflect.Slice:
		return fmt.Sprintf("[]%s",
			typeToString(t.Elem(), types, errs),
		)
	case reflect.Array:
		return fmt.Sprintf("[%d]%s",
			t.Len(),
			typeToString(t.Elem(), types, errs),
		)
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s",
			typeToString(t.Key(), types, errs),
			typeToString(t.Elem(), types, errs),
		)
	case reflect.Struct:
		// continue
	default:
		return t.Kind().String()
	}
	interfaces := checkInterfaces(t)
	if len(interfaces) > 0 {
		return "(" + strings.Join(interfaces, ",") + ")"
	}

	fields, e := structFields(t)
	if errs != nil && len(e) > 0 {
		*errs = append(*errs, e...)
	}

	fieldMap := map[string]reflect.StructField{}
	keys := []string{}
	for _, f := range fields {
		if !f.IsExported() {
			continue
		}
		if !isConcrete(baseType(f.Type)) {
			continue
		}
		name := f.Name
		if f.Anonymous {
			name = "." + name
		}
		if _, ok := fieldMap[name]; !ok {
			keys = append(keys, name)
		}
		fieldMap[name] = f
	}
	sort.Strings(keys)

	r := "{ "
	n := len(keys)
	if n == 0 {
		*errs = append(*errs, fmt.Errorf("%w %s", ErrEmptyStruct, t))
	}
	for i, name := range keys {
		f := fieldMap[name]
		if !f.Anonymous {
			r += f.Name + ":"
		}
		if tag := f.Tag.Get("reflect"); tag != "" {
			r += tag
		} else {
			r += typeToString(f.Type, types, errs)
		}
		if i < n-1 {
			r += ", "
		}
	}
	return r + " }"
}
