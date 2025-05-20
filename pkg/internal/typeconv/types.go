package typeconv

import "strings"

// canonicalType normalizes SQL types for comparison.
func CanonicalType(typ string) string {
	t := strings.ToUpper(typ)
	switch t {
	case "INT4", "INT8", "INTEGER":
		return "INTEGER"
	case "BOOL", "BOOLEAN":
		return "BOOLEAN"
	case "TEXT":
		return "TEXT"
	case "REAL", "FLOAT4", "FLOAT8":
		return "REAL"
	case "TIMESTAMP", "TIMESTAMPTZ":
		return "TIMESTAMP"
	case "UUID":
		return "UUID"
	default:
		return t
	}
}

func MapGoTypeToSQL(goType string) string {
	switch goType {
	case "int", "int32", "int64":
		return "INTEGER"
	case "string":
		return "TEXT"
	case "bool":
		return "BOOLEAN"
	case "float32", "float64":
		return "REAL"
	case "time.Time":
		return "TIMESTAMP"
	default:
		return "TEXT"
	}
}
