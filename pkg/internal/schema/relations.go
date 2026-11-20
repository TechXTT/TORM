package schema

import (
	"slices"
	"strings"
	"text/scanner"
)

func parseRelation(target *Model, source fieldSyntax) (*Relation, error) {
	r := &Relation{
		position: source.position,
		Name:     source.name,
		Target:   target,
		Many:     source.list,
		Optional: source.optional,
	}
	attrs, err := attributes(source.attributes, "relation")
	if err != nil {
		return nil, err
	}
	attr, ok := attrs["relation"]
	if !ok {
		return r, nil
	}
	args, named, err := splitArgs(attr, "name", "fields", "references", "onDelete", "onUpdate", "map")
	if err != nil {
		return nil, err
	}
	if len(args) > 1 {
		return nil, diagnostic(attr.position, "@relation accepts at most one positional name")
	}
	if len(args) == 1 {
		if _, ok := named["name"]; ok {
			return nil, diagnostic(attr.position, "duplicate relation name")
		}
		named["name"] = args[0]
	}
	if label, ok := named["name"]; ok {
		r.label, err = physicalName(label)
		if err != nil {
			return nil, err
		}
	}
	local, owns := named["fields"]
	remote, references := named["references"]
	if owns != references || (owns && source.list) {
		return nil, diagnostic(attr.position, "fields and references must be specified together on a singular relation")
	}
	if !owns {
		if len(named) > 1 || (len(named) == 1 && r.label == "") {
			return nil, diagnostic(attr.position, "relation without fields and references accepts only a name")
		}
		return r, nil
	}
	r.fieldList = &local
	r.referenceList = &remote
	if mapped, ok := named["map"]; ok {
		r.Constraint, err = physicalName(mapped)
		if err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"onDelete", "onUpdate"} {
		action, ok := named[key]
		if !ok {
			continue
		}
		sql, ok := referentialActions[action.value]
		if !ok || action.kind != scanner.Ident || action.call {
			return nil, diagnostic(action.position, "invalid referential action %q", action.value)
		}
		if key == "onDelete" {
			r.OnDelete = sql
		} else {
			r.OnUpdate = sql
		}
	}
	return r, nil
}

var referentialActions = map[string]string{
	"Cascade": "CASCADE", "Restrict": "RESTRICT", "NoAction": "NO ACTION",
	"SetNull": "SET NULL", "SetDefault": "SET DEFAULT",
}

func (m *Model) isUnique(fields []*Field) bool {
	for _, key := range m.Keys {
		if len(key.Fields) != len(fields) {
			continue
		}
		matches := true
		for _, field := range fields {
			matches = matches && slices.Contains(key.Fields, field)
		}
		if matches {
			return true
		}
	}
	return false
}

func (m *Model) simpleID() *Field {
	for _, key := range m.Keys {
		if key.Primary && len(key.Fields) == 1 {
			return key.Fields[0]
		}
	}
	return nil
}

func resolveRelations(s *Schema, tables map[string]bool) error {
	type relationKey struct {
		from, to *Model
		label    string
	}
	directions := make(map[relationKey][]*Relation)
	for _, model := range s.Models {
		for _, relation := range model.Relations {
			key := relationKey{model, relation.Target, relation.label}
			directions[key] = append(directions[key], relation)
		}
	}
	resolved := make(map[*Relation]bool)
	for _, model := range s.Models {
		for _, r := range model.Relations {
			if resolved[r] {
				continue
			}
			var reverse *Relation

			for _, candidate := range directions[relationKey{r.Target, model, r.label}] {
				if candidate == r {
					continue
				}
				if reverse != nil || resolved[candidate] {
					return diagnostic(r.position, "ambiguous relation %s.%s, give each relation a distinct name", model.Name, r.Name)
				}
				reverse = candidate
			}
			if reverse == nil {
				return diagnostic(r.position, "relation %s.%s requires a matching field on %s", model.Name, r.Name, r.Target.Name)
			}
			if r.Many && reverse.Many {
				if err := implicitJoin(s, model, r, reverse, tables); err != nil {
					return err
				}
			} else {
				owner, side := r, model
				inverse := reverse
				if r.fieldList == nil {
					owner, side, inverse = reverse, r.Target, r
				}
				if owner.fieldList == nil || inverse.fieldList != nil {
					return diagnostic(r.position, "exactly one side of a relation must define fields and references")
				}
				if err := resolveForeignKey(side, owner, inverse); err != nil {
					return err
				}
			}
			resolved[r], resolved[reverse] = true, true
		}
	}
	for _, model := range s.Models {
		constraints := make(map[string]bool)
		for _, key := range model.Keys {
			constraints[key.Name] = true
		}
		for _, relation := range model.Relations {
			if relation.Constraint == "" {
				continue
			}
			if constraints[relation.Constraint] {
				return diagnostic(relation.position, "duplicate constraint %s", relation.Constraint)
			}
			constraints[relation.Constraint] = true
		}
	}
	return nil
}

func resolveForeignKey(model *Model, r, inverse *Relation) error {
	local, err := model.fields(*r.fieldList)
	if err != nil {
		return err
	}
	remote, err := r.Target.fields(*r.referenceList)
	if err != nil {
		return err
	}
	if len(local) != len(remote) || !r.Target.isUnique(remote) {
		return diagnostic(r.position, "references must match the foreign-key fields and target a unique key")
	}
	var nullable bool

	for i, field := range local {
		if field.List || remote[i].List || field.SQLType != remote[i].SQLType {
			return diagnostic(r.position, "%s and %s must use the same PostgreSQL scalar type", field.Name, remote[i].Name)
		}
		nullable = nullable || field.Optional
	}
	if r.Optional != nullable {
		return diagnostic(r.position, "relation optionality must match its foreign-key fields")
	}
	if !inverse.Many && (!model.isUnique(local) || !inverse.Optional) {
		return diagnostic(inverse.position, "a one-to-one relation requires a unique foreign key and an optional inverse")
	}
	r.Local, r.Remote = local, remote
	inverse.Local, inverse.Remote = remote, local
	if r.Constraint == "" {
		r.Constraint = Name(model.Table, strings.Join(columns(local), "_"), "fkey")
	}
	if r.OnDelete == "" {
		r.OnDelete = "RESTRICT"
		if nullable {
			r.OnDelete = "SET NULL"
		}
	}
	if r.OnUpdate == "" {
		r.OnUpdate = "CASCADE"
	}
	for _, action := range []string{r.OnDelete, r.OnUpdate} {
		for _, field := range local {
			if (action == "SET NULL" && !field.Optional) || (action == "SET DEFAULT" && !field.Optional && field.Default == "") {
				return diagnostic(r.position, "%s cannot be used with foreign-key field %s", action, field.Name)
			}
		}
	}
	return nil
}

func implicitJoin(s *Schema, model *Model, relation, reverse *Relation, tables map[string]bool) error {
	a, b, aRel, bRel := model, relation.Target, relation, reverse
	if a.Name > b.Name || (a == b && aRel.Name > bRel.Name) {
		a, b, aRel, bRel = b, a, bRel, aRel
	}
	if a == b && aRel.label == "" {
		return diagnostic(aRel.position, "implicit self-relations require a name")
	}
	aKey, bKey := a.simpleID(), b.simpleID()
	if aKey == nil || bKey == nil {
		return diagnostic(aRel.position, "implicit many-to-many requires a single primary key on both models")
	}
	name := "_" + a.Name + "To" + b.Name
	if aRel.label != "" {
		name = "_" + aRel.label
	}
	if len(name) > 63 || tables[name] {
		return diagnostic(aRel.position, "invalid or duplicate implicit relation table %s", name)
	}
	tables[name] = true
	join := Join{Table: name, A: a, B: b, AKey: aKey, BKey: bKey}
	s.Joins = append(s.Joins, join)
	aRel.Local, aRel.Remote = []*Field{aKey}, []*Field{bKey}
	bRel.Local, bRel.Remote = []*Field{bKey}, []*Field{aKey}
	aRel.Join, bRel.Join = &join, &join
	aRel.LinkLocal, aRel.LinkRemote = "A", "B"
	bRel.LinkLocal, bRel.LinkRemote = "B", "A"
	return nil
}
