package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
)

type Relation struct {
	Target     []string
	Types      []string
	JoinTable  string
	JoinParent []string
	JoinTarget []string
}

func (s Store[M]) LoadRelated(ctx context.Context, relation Relation, parents [][]any) ([][]*M, error) {
	result := make([][]*M, len(parents))
	for i := range result {
		result[i] = []*M{}
	}
	const batchSize = 256
	width := len(relation.Target)
	if width == 0 || width != len(relation.Types) {
		return nil, fmt.Errorf("invalid relation metadata")
	}
	linked := relation.JoinTable != ""
	if linked && (width != len(relation.JoinTarget) || width != len(relation.JoinParent)) {
		return nil, fmt.Errorf("invalid join table metadata")
	}
	aliases := make([]string, width)
	matches := make([]string, width)
	from := s.table() + " AS t"
	var join []string

	for i, target := range relation.Target {
		key := "t." + schema.Quote(target)
		if linked {
			join = append(join, key+" = j."+schema.Quote(relation.JoinTarget[i]))
			key = "j." + schema.Quote(relation.JoinParent[i])
		}
		aliases[i] = "key" + strconv.Itoa(i)
		matches[i] = key + " = k." + aliases[i]
	}
	if linked {
		from += ` JOIN "public".` + schema.Quote(relation.JoinTable) + " AS j ON " + strings.Join(join, " AND ")
	}
	prefix := "SELECT " + s.columns("t.") + ", k.ordinal FROM " + from + " JOIN (VALUES "
	suffix := ") AS k(ordinal, " + strings.Join(aliases, ", ") + ") ON " + strings.Join(matches, " AND ")
	for start := 0; start < len(parents); start += batchSize {
		end := min(start+batchSize, len(parents))
		args := make([]any, 0, (end-start)*width)
		var query strings.Builder

		query.WriteString(prefix)
		for i := start; i < end; i++ {
			if len(parents[i]) != width {
				return nil, fmt.Errorf("relation key %d has incorrect arity", i)
			}
			if i != start {
				query.WriteString(", ")
			}
			query.WriteByte('(')
			query.WriteString(strconv.Itoa(i))
			for j, value := range parents[i] {
				args = append(args, bind(value, false))
				query.WriteString(", $")
				query.WriteString(strconv.Itoa(len(args)))
				query.WriteString("::")
				query.WriteString(relation.Types[j])
			}
			query.WriteByte(')')
		}
		query.WriteString(suffix)
		if err := s.collectRelated(ctx, query.String(), args, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s Store[M]) collectRelated(ctx context.Context, query string, args []any, result [][]*M) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ordinal int

		record, err := s.model.Scan(rows, &ordinal)
		if err != nil {
			return err
		}
		result[ordinal] = append(result[ordinal], record)
	}
	return rows.Err()
}

func Link(ctx context.Context, db Queryer, table, fromColumn, toColumn string, from, to any, connect bool) error {
	relation := `"public".` + schema.Quote(table)
	first, second := schema.Quote(fromColumn), schema.Quote(toColumn)
	query := "DELETE FROM " + relation + " WHERE " + first + " = $1 AND " + second + " = $2"
	if connect {
		query = "INSERT INTO " + relation + " (" + first + ", " + second + ") VALUES ($1, $2) ON CONFLICT DO NOTHING"
	}
	_, err := db.ExecContext(ctx, query, from, to)
	return err
}
