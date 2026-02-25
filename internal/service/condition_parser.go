package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ConditionParser evaluates DSL condition expressions against a message.
//
// Supported functions:
//   - len(message)              → rune count
//   - contains(message, "str")  → substring check
//   - matches(message, "regex") → regex match
//   - has_code_block(message)   → triple-backtick detection
//   - count(message, "str")     → occurrence count
//
// Supported operators: AND, OR, NOT, parentheses
// Comparison operators: >, <, >=, <=, ==
type ConditionParser struct {
	codeBlockRe *regexp.Regexp
}

// NewConditionParser creates a new ConditionParser.
func NewConditionParser() *ConditionParser {
	return &ConditionParser{
		codeBlockRe: regexp.MustCompile("(?s)```"),
	}
}

// Evaluate evaluates a condition expression against a message.
// Empty or whitespace-only conditions always return true.
func (p *ConditionParser) Evaluate(condition, message string) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true, nil
	}

	tokens, err := p.tokenize(condition)
	if err != nil {
		return false, fmt.Errorf("tokenize condition: %w", err)
	}

	pos := 0
	result, err := p.parseOr(tokens, &pos, message)
	if err != nil {
		return false, err
	}

	if pos < len(tokens) {
		return false, fmt.Errorf("unexpected token at position %d: %s", pos, tokens[pos])
	}

	return result, nil
}

// Token types for the condition DSL.
type tokenKind int

const (
	tkFunc   tokenKind = iota // function call: len, contains, matches, etc.
	tkNum                     // numeric literal
	tkStr                     // string literal
	tkOp                      // comparison operator: >, <, >=, <=, ==
	tkAnd                     // AND
	tkOr                      // OR
	tkNot                     // NOT
	tkLParen                  // (
	tkRParen                  // )
	tkComma                   // ,
	tkIdent                   // identifier (e.g., "message")
)

type token struct {
	kind  tokenKind
	value string
}

func (t token) String() string { return t.value }

// tokenize splits a condition string into tokens.
func (p *ConditionParser) tokenize(condition string) ([]token, error) {
	var tokens []token
	i := 0
	runes := []rune(condition)

	for i < len(runes) {
		ch := runes[i]

		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}

		// Parentheses
		if ch == '(' {
			tokens = append(tokens, token{tkLParen, "("})
			i++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, token{tkRParen, ")"})
			i++
			continue
		}

		// Comma
		if ch == ',' {
			tokens = append(tokens, token{tkComma, ","})
			i++
			continue
		}

		// String literal
		if ch == '"' || ch == '\'' {
			str, end, err := p.readString(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{tkStr, str})
			i = end
			continue
		}

		// Number
		if ch >= '0' && ch <= '9' {
			num, end := p.readNumber(runes, i)
			tokens = append(tokens, token{tkNum, num})
			i = end
			continue
		}

		// Comparison operators
		if ch == '>' || ch == '<' || ch == '=' || ch == '!' {
			op, end := p.readOperator(runes, i)
			if op != "" {
				tokens = append(tokens, token{tkOp, op})
				i = end
				continue
			}
		}

		// Identifiers and keywords
		if isIdentStart(ch) {
			word, end := p.readIdent(runes, i)
			i = end

			switch strings.ToUpper(word) {
			case "AND":
				tokens = append(tokens, token{tkAnd, "AND"})
			case "OR":
				tokens = append(tokens, token{tkOr, "OR"})
			case "NOT":
				tokens = append(tokens, token{tkNot, "NOT"})
			default:
				// Check if it's a function (followed by '(')
				j := i
				for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t') {
					j++
				}
				if j < len(runes) && runes[j] == '(' {
					tokens = append(tokens, token{tkFunc, word})
				} else {
					tokens = append(tokens, token{tkIdent, word})
				}
			}
			continue
		}

		return nil, fmt.Errorf("unexpected character at position %d: %c", i, ch)
	}

	return tokens, nil
}

func isIdentStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch rune) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func (p *ConditionParser) readString(runes []rune, start int) (string, int, error) {
	quote := runes[start]
	i := start + 1
	var sb strings.Builder

	for i < len(runes) {
		ch := runes[i]
		if ch == quote {
			return sb.String(), i + 1, nil
		}
		if ch == '\\' && i+1 < len(runes) {
			// Handle escape sequences
			next := runes[i+1]
			switch next {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			default:
				sb.WriteRune(next)
			}
			i += 2
			continue
		}
		sb.WriteRune(ch)
		i++
	}

	return "", 0, fmt.Errorf("unclosed string starting at position %d", start)
}

func (p *ConditionParser) readNumber(runes []rune, start int) (string, int) {
	i := start
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	return string(runes[start:i]), i
}

func (p *ConditionParser) readOperator(runes []rune, start int) (string, int) {
	if start+1 < len(runes) {
		two := string(runes[start : start+2])
		if two == ">=" || two == "<=" || two == "==" || two == "!=" {
			return two, start + 2
		}
	}
	ch := runes[start]
	if ch == '>' || ch == '<' {
		return string(ch), start + 1
	}
	return "", start
}

func (p *ConditionParser) readIdent(runes []rune, start int) (string, int) {
	i := start
	for i < len(runes) && isIdentChar(runes[i]) {
		i++
	}
	return string(runes[start:i]), i
}

// Recursive descent parser: OR → AND → NOT → primary

// parseOr handles OR expressions (lowest precedence).
func (p *ConditionParser) parseOr(tokens []token, pos *int, msg string) (bool, error) {
	left, err := p.parseAnd(tokens, pos, msg)
	if err != nil {
		return false, err
	}

	for *pos < len(tokens) && tokens[*pos].kind == tkOr {
		*pos++ // consume OR
		right, err := p.parseAnd(tokens, pos, msg)
		if err != nil {
			return false, err
		}
		left = left || right
	}

	return left, nil
}

// parseAnd handles AND expressions.
func (p *ConditionParser) parseAnd(tokens []token, pos *int, msg string) (bool, error) {
	left, err := p.parseNot(tokens, pos, msg)
	if err != nil {
		return false, err
	}

	for *pos < len(tokens) && tokens[*pos].kind == tkAnd {
		*pos++ // consume AND
		right, err := p.parseNot(tokens, pos, msg)
		if err != nil {
			return false, err
		}
		left = left && right
	}

	return left, nil
}

// parseNot handles NOT expressions (highest precedence among logical ops).
func (p *ConditionParser) parseNot(tokens []token, pos *int, msg string) (bool, error) {
	if *pos < len(tokens) && tokens[*pos].kind == tkNot {
		*pos++ // consume NOT
		val, err := p.parseNot(tokens, pos, msg) // NOT is right-associative
		if err != nil {
			return false, err
		}
		return !val, nil
	}
	return p.parsePrimary(tokens, pos, msg)
}

// parsePrimary handles function calls, parenthesized expressions, and comparisons.
func (p *ConditionParser) parsePrimary(tokens []token, pos *int, msg string) (bool, error) {
	if *pos >= len(tokens) {
		return false, fmt.Errorf("unexpected end of expression")
	}

	tok := tokens[*pos]

	// Parenthesized expression
	if tok.kind == tkLParen {
		*pos++ // consume (
		result, err := p.parseOr(tokens, pos, msg)
		if err != nil {
			return false, err
		}
		if *pos >= len(tokens) || tokens[*pos].kind != tkRParen {
			return false, fmt.Errorf("expected closing parenthesis")
		}
		*pos++ // consume )
		return result, nil
	}

	// Function call
	if tok.kind == tkFunc {
		return p.evalFunction(tokens, pos, msg)
	}

	return false, fmt.Errorf("unexpected token: %s", tok.value)
}

// evalFunction evaluates a function call and optional comparison.
func (p *ConditionParser) evalFunction(tokens []token, pos *int, msg string) (bool, error) {
	funcName := tokens[*pos].value
	*pos++ // consume function name

	// Expect '('
	if *pos >= len(tokens) || tokens[*pos].kind != tkLParen {
		return false, fmt.Errorf("expected '(' after function %s", funcName)
	}
	*pos++ // consume (

	// Read arguments
	args, err := p.readFuncArgs(tokens, pos, msg)
	if err != nil {
		return false, fmt.Errorf("function %s: %w", funcName, err)
	}

	// Expect ')'
	if *pos >= len(tokens) || tokens[*pos].kind != tkRParen {
		return false, fmt.Errorf("expected ')' after function %s arguments", funcName)
	}
	*pos++ // consume )

	// Evaluate function
	switch funcName {
	case "len":
		numVal := utf8.RuneCountInString(msg)
		return p.evalComparison(tokens, pos, numVal)

	case "contains":
		if len(args) < 1 {
			return false, fmt.Errorf("contains() requires a search string argument")
		}
		return strings.Contains(msg, args[0]), nil

	case "matches":
		if len(args) < 1 {
			return false, fmt.Errorf("matches() requires a regex pattern argument")
		}
		re, err := regexp.Compile(args[0])
		if err != nil {
			return false, fmt.Errorf("invalid regex in matches(): %w", err)
		}
		return re.MatchString(msg), nil

	case "has_code_block":
		// Count occurrences of ``` — need at least 2 (open + close)
		count := strings.Count(msg, "```")
		return count >= 2, nil

	case "count":
		if len(args) < 1 {
			return false, fmt.Errorf("count() requires a search string argument")
		}
		numVal := strings.Count(msg, args[0])
		return p.evalComparison(tokens, pos, numVal)

	default:
		return false, fmt.Errorf("unknown function: %s", funcName)
	}
}

// readFuncArgs reads function arguments (string literals), skipping "message" identifiers.
func (p *ConditionParser) readFuncArgs(tokens []token, pos *int, _ string) ([]string, error) {
	var args []string

	for *pos < len(tokens) && tokens[*pos].kind != tkRParen {
		tok := tokens[*pos]

		if tok.kind == tkIdent && tok.value == "message" {
			*pos++
			// Skip comma after "message"
			if *pos < len(tokens) && tokens[*pos].kind == tkComma {
				*pos++
			}
			continue
		}

		if tok.kind == tkStr {
			args = append(args, tok.value)
			*pos++
			// Skip comma
			if *pos < len(tokens) && tokens[*pos].kind == tkComma {
				*pos++
			}
			continue
		}

		// Unexpected token in function args
		break
	}

	return args, nil
}

// evalComparison evaluates a comparison operator against a numeric value.
// If no comparison follows, returns false (numeric functions need comparison).
func (p *ConditionParser) evalComparison(tokens []token, pos *int, numVal int) (bool, error) {
	if *pos >= len(tokens) || tokens[*pos].kind != tkOp {
		return false, fmt.Errorf("expected comparison operator after numeric function")
	}

	op := tokens[*pos].value
	*pos++ // consume operator

	if *pos >= len(tokens) || tokens[*pos].kind != tkNum {
		return false, fmt.Errorf("expected number after operator %s", op)
	}

	rhs, err := strconv.Atoi(tokens[*pos].value)
	if err != nil {
		return false, fmt.Errorf("invalid number: %s", tokens[*pos].value)
	}
	*pos++ // consume number

	switch op {
	case ">":
		return numVal > rhs, nil
	case "<":
		return numVal < rhs, nil
	case ">=":
		return numVal >= rhs, nil
	case "<=":
		return numVal <= rhs, nil
	case "==":
		return numVal == rhs, nil
	case "!=":
		return numVal != rhs, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", op)
	}
}
