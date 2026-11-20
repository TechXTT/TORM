package runtime

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type arrayElement struct {
	value []byte
}

func (e *arrayElement) Scan(src any) error {
	switch value := src.(type) {
	case []byte:
		e.value = value
	case string:
		e.value = []byte(value)
	default:
		return fmt.Errorf("unsupported or NULL PostgreSQL array element")
	}
	return nil
}

type ArrayDestination[T any] struct {
	Values *[]T
}

func (a ArrayDestination[T]) Scan(src any) error {
	var elements []arrayElement

	if err := pq.Array(&elements).Scan(src); err != nil {
		return err
	}
	values := make([]T, len(elements))
	for i, element := range elements {
		if err := parseElement(element.value, &values[i]); err != nil {
			return fmt.Errorf("array element %d: %w", i, err)
		}
	}
	*a.Values = values
	return nil
}

func parseElement[T any](raw []byte, dest *T) error {
	switch target := any(dest).(type) {
	case sql.Scanner:
		return target.Scan(raw)
	case *time.Time:
		value := string(raw)
		parsed, err := pq.ParseTimestamp(time.UTC, value)
		if err == nil {
			*target = parsed
			return nil
		}
		for _, layout := range []string{"15:04:05.999999999Z07:00:00", "15:04:05.999999999Z07:00", "15:04:05.999999999Z07", "15:04:05.999999999"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				*target = parsed
				return nil
			}
		}
		return err
	case *json.RawMessage:
		*target = append((*target)[:0], raw...)
		return nil
	case *[]byte:
		value, ok := strings.CutPrefix(string(raw), `\x`)
		if !ok {
			return fmt.Errorf("bytea array requires PostgreSQL hex output")
		}
		decoded, err := hex.DecodeString(value)
		*target = decoded
		return err
	}
	value := reflect.ValueOf(dest).Elem()
	switch value.Kind() {
	case reflect.String:
		value.SetString(string(raw))
	case reflect.Bool:
		parsed, err := strconv.ParseBool(string(raw))
		if err != nil {
			return err
		}
		value.SetBool(parsed)
	case reflect.Int, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(string(raw), 10, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetInt(parsed)
	case reflect.Float64:
		parsed, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return err
		}
		value.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported PostgreSQL array element type %T", *dest)
	}
	return nil
}
