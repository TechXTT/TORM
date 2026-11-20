package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/scanner"
)

type position = scanner.Position

type expression struct {
	position
	name  string
	value string
	kind  rune
	args  []expression
	list  bool
	call  bool
}

type attribute struct {
	position
	name string
	args []expression
}

type fieldSyntax struct {
	position
	name       string
	kind       string
	optional   bool
	list       bool
	attributes []attribute
}

type block struct {
	position
	kind       string
	name       string
	fields     []fieldSyntax
	attributes []attribute
	settings   map[string]expression
}

type parser struct {
	scanner scanner.Scanner
	token   rune
	value   string
	pos     position
	err     error
}

func (p *parser) next() {
	p.token = p.scanner.Scan()
	p.value = p.scanner.TokenText()
	p.pos = p.scanner.Position
}

func (p *parser) lines() {
	for p.token == '\n' {
		p.next()
	}
}

func (p *parser) require(token rune) error {
	if p.token != token {
		return diagnostic(p.pos, "expected %s, found %q", scanner.TokenString(token), p.value)
	}
	p.next()
	return nil
}

func (p *parser) identifier() (string, error) {
	name := p.value
	return name, p.require(scanner.Ident)
}

func diagnostic(pos position, format string, args ...interface{}) error {
	return fmt.Errorf("%s: %s", pos, fmt.Sprintf(format, args...))
}

func (p *parser) expressions(end rune) ([]expression, error) {
	var values []expression

	p.lines()
	for p.token != end {
		value, err := p.expression()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		p.lines()
		if p.token == end {
			break
		}
		if err := p.require(','); err != nil {
			return nil, err
		}
		p.lines()
	}
	p.next()
	return values, nil
}

func (p *parser) expression() (expression, error) {
	value := expression{position: p.pos, value: p.value, kind: p.token}
	switch p.token {
	case '[':
		p.next()
		value.list = true
		args, err := p.expressions(']')
		value.args = args
		return value, err
	case scanner.String:
		raw, err := strconv.Unquote(p.value)
		if err != nil {
			return value, diagnostic(p.pos, "invalid string: %v", err)
		}
		value.value = raw
		p.next()
		return value, nil
	case '-':
		p.next()
		if p.token != scanner.Int && p.token != scanner.Float {
			return value, diagnostic(p.pos, "expected a number after -")
		}
		value.kind = p.token
		value.value += p.value
		p.next()
		return value, nil
	case scanner.Int, scanner.Float:
		p.next()
		return value, nil
	case scanner.Ident:
		p.next()
		switch p.token {
		case ':':
			p.next()
			p.lines()
			named, err := p.expression()
			if err != nil {
				return value, err
			}
			if named.name != "" {
				return value, diagnostic(value.position, "unexpected nested named argument")
			}
			named.name = value.value
			return named, nil
		case '(':
			p.next()
			value.call = true
			args, err := p.expressions(')')
			value.args = args
			return value, err
		default:
			return value, nil
		}
	default:
		return value, diagnostic(p.pos, "expected an expression, found %q", p.value)
	}
}

func (p *parser) attribute() (attribute, error) {
	attr := attribute{position: p.pos}
	name, err := p.identifier()
	if err != nil {
		return attr, err
	}
	attr.name = name
	if p.token == '.' {
		p.next()
		suffix, err := p.identifier()
		if err != nil {
			return attr, err
		}
		attr.name += "." + suffix
	}
	if p.token != '(' {
		return attr, nil
	}
	p.next()
	attr.args, err = p.expressions(')')
	return attr, err
}

func (p *parser) block() (block, error) {
	result := block{position: p.pos}
	kind, err := p.identifier()
	if err != nil {
		return result, err
	}
	result.kind = kind
	switch kind {
	case "model", "enum", "datasource", "generator":
	default:
		return result, diagnostic(result.position, "unsupported block %q", kind)
	}
	result.name, err = p.identifier()
	if err != nil {
		return result, err
	}
	if err := p.require('{'); err != nil {
		return result, err
	}
	result.settings = make(map[string]expression)
	p.lines()
	for p.token != '}' {
		if kind == "generator" || kind == "datasource" {
			key, err := p.identifier()
			if err != nil {
				return result, err
			}
			if err := p.require('='); err != nil {
				return result, err
			}
			value, err := p.expression()
			if err != nil {
				return result, err
			}
			if _, found := result.settings[key]; found {
				return result, diagnostic(value.position, "duplicate setting %q", key)
			}
			result.settings[key] = value
		} else if p.token == '@' {
			p.next()
			if err := p.require('@'); err != nil {
				return result, err
			}
			attr, err := p.attribute()
			if err != nil {
				return result, err
			}
			result.attributes = append(result.attributes, attr)
		} else {
			field := fieldSyntax{position: p.pos}
			field.name, err = p.identifier()
			if err != nil {
				return result, err
			}
			if kind == "model" {
				field.kind, err = p.identifier()
				if err != nil {
					return result, err
				}
				switch p.token {
				case '?':
					field.optional = true
					p.next()
				case '[':
					field.list = true
					p.next()
					if err := p.require(']'); err != nil {
						return result, err
					}
				}
			}
			for p.token == '@' {
				p.next()
				attr, err := p.attribute()
				if err != nil {
					return result, err
				}
				field.attributes = append(field.attributes, attr)
			}
			result.fields = append(result.fields, field)
		}
		if p.token != '}' && p.token != '\n' {
			return result, diagnostic(p.pos, "expected a newline or }, found %q", p.value)
		}
		p.lines()
	}
	p.next()
	return result, nil
}

func parse(path string, input string) ([]block, error) {
	var p parser
	var blocks []block

	p.scanner.Init(strings.NewReader(input))
	p.scanner.Filename = path
	p.scanner.Whitespace &^= 1 << '\n'
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats | scanner.ScanComments | scanner.SkipComments
	p.scanner.Error = func(s *scanner.Scanner, message string) {
		if p.err == nil {
			p.err = diagnostic(s.Position, "%s", message)
		}
	}
	p.next()
	p.lines()
	for p.token != scanner.EOF {
		entry, err := p.block()
		if p.err != nil {
			return nil, p.err
		}
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, entry)
		p.lines()
	}
	return blocks, p.err
}

func Load(path string, legacyNames bool) (*Schema, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	paths := []string{path}
	if info.IsDir() {
		paths = nil
		err := filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && filepath.Ext(entryPath) == ".prisma" {
				paths = append(paths, entryPath)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	var blocks []block
	for _, file := range paths {
		input, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		parsed, err := parse(file, string(input))
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, parsed...)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%s: no schema declarations found", path)
	}
	return resolve(blocks, legacyNames)
}
