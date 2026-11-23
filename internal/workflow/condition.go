package workflow

// 边条件的表达式求值。条件只读事实，不改事实：事实由运行事实写进来，
// 条件只负责判断一条边这次能不能走。

import (
	"fmt"
	"strings"
)

// Condition 是编译后的条件表达式。
type Condition struct {
	root       conditionNode
	expression string
}

// Text 返回原始表达式。保存定义时留原文，界面也照原文显示。
func (c *Condition) Text() string {
	if c == nil {
		return ""
	}
	return c.expression
}

// Eval 用事实判断条件是否成立。缺失的事实当作空值。
func (c *Condition) Eval(facts map[string]string) bool {
	if c == nil {
		return true
	}
	return c.root.eval(facts)
}

// conditionNode 是条件树的一个节点。
type conditionNode interface {
	eval(facts map[string]string) bool
}

// conditionValue 是一个可比较的操作数：事实路径或字面量。
type conditionValue struct {
	path    string
	literal string
	isPath  bool
}

func (v conditionValue) resolve(facts map[string]string) string {
	if !v.isPath {
		return v.literal
	}
	return facts[v.path]
}

func (v conditionValue) truthy(facts map[string]string) bool {
	return truthy(v.resolve(facts))
}

// truthy 判断一个值是不是“成立”。空串、false 和 0 都不成立。
func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "0", "null", "no":
		return false
	}
	return true
}

// conditionNot 取反。
type conditionNot struct{ inner conditionNode }

func (n conditionNot) eval(facts map[string]string) bool { return !n.inner.eval(facts) }

// conditionAnd 要求两边都成立。
type conditionAnd struct{ left, right conditionNode }

func (n conditionAnd) eval(facts map[string]string) bool {
	return n.left.eval(facts) && n.right.eval(facts)
}

// conditionOr 要求有一边成立。
type conditionOr struct{ left, right conditionNode }

func (n conditionOr) eval(facts map[string]string) bool {
	return n.left.eval(facts) || n.right.eval(facts)
}

// conditionTruthy 是单个操作数的真假判断。
type conditionTruthy struct{ value conditionValue }

func (n conditionTruthy) eval(facts map[string]string) bool { return n.value.truthy(facts) }

// conditionCompare 比较两个操作数。
type conditionCompare struct {
	left    conditionValue
	right   conditionValue
	negated bool
}

func (n conditionCompare) eval(facts map[string]string) bool {
	equal := n.left.resolve(facts) == n.right.resolve(facts)
	if n.negated {
		return !equal
	}
	return equal
}

// ParseCondition 解析条件表达式。语法很小：比较、与、或、取反和括号。
// 事实路径是点分段的名字，例如 phase.review.status。
func ParseCondition(expression string) (*Condition, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, fmt.Errorf("the condition is empty")
	}
	tokens, err := tokenizeCondition(trimmed)
	if err != nil {
		return nil, err
	}
	parser := &conditionParser{tokens: tokens}
	node, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	if !parser.done() {
		return nil, fmt.Errorf("the condition has extra text after %q", parser.peek().text)
	}
	return &Condition{root: node, expression: trimmed}, nil
}

// conditionToken 是条件表达式的记号。
type conditionToken struct {
	kind   string
	text   string
	quoted bool
}

const (
	tokenAnd   = "and"
	tokenOr    = "or"
	tokenEqual = "equal"
	tokenNotEq = "notEqual"
	tokenNot   = "not"
	tokenOpen  = "open"
	tokenClose = "close"
	tokenValue = "value"
)

// tokenizeCondition 把表达式切成记号。路径与字面量的区别只在后面取值时才有。
func tokenizeCondition(expression string) ([]conditionToken, error) {
	var tokens []conditionToken
	runes := []rune(expression)
	for index := 0; index < len(runes); {
		current := runes[index]
		switch {
		case current == ' ' || current == '\t' || current == '\n' || current == '\r':
			index++
		case current == '&':
			if index+1 >= len(runes) || runes[index+1] != '&' {
				return nil, fmt.Errorf("the condition uses a single &; write &&")
			}
			tokens = append(tokens, conditionToken{kind: tokenAnd, text: "&&"})
			index += 2
		case current == '|':
			if index+1 >= len(runes) || runes[index+1] != '|' {
				return nil, fmt.Errorf("the condition uses a single |; write ||")
			}
			tokens = append(tokens, conditionToken{kind: tokenOr, text: "||"})
			index += 2
		case current == '=':
			if index+1 >= len(runes) || runes[index+1] != '=' {
				return nil, fmt.Errorf("the condition uses a single =; write ==")
			}
			tokens = append(tokens, conditionToken{kind: tokenEqual, text: "=="})
			index += 2
		case current == '!':
			if index+1 < len(runes) && runes[index+1] == '=' {
				tokens = append(tokens, conditionToken{kind: tokenNotEq, text: "!="})
				index += 2
				continue
			}
			tokens = append(tokens, conditionToken{kind: tokenNot, text: "!"})
			index++
		case current == '(':
			tokens = append(tokens, conditionToken{kind: tokenOpen, text: "("})
			index++
		case current == ')':
			tokens = append(tokens, conditionToken{kind: tokenClose, text: ")"})
			index++
		case current == '"' || current == '\'':
			value, next, err := readQuoted(runes, index, current)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, conditionToken{kind: tokenValue, text: value, quoted: true})
			index = next
		default:
			value, next, err := readPath(runes, index)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, conditionToken{kind: tokenValue, text: value})
			index = next
		}
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("the condition is empty")
	}
	return tokens, nil
}

// readQuoted 读取一段引号里的字面量。
func readQuoted(runes []rune, start int, quote rune) (string, int, error) {
	var builder strings.Builder
	for index := start + 1; index < len(runes); index++ {
		switch runes[index] {
		case '\\':
			if index+1 < len(runes) {
				builder.WriteRune(runes[index+1])
				index++
				continue
			}
		case quote:
			return builder.String(), index + 1, nil
		default:
			builder.WriteRune(runes[index])
		}
	}
	return "", 0, fmt.Errorf("the condition has an unterminated quote")
}

// readPath 读取一段事实路径或裸字面量。
func readPath(runes []rune, start int) (string, int, error) {
	index := start
	for index < len(runes) {
		current := runes[index]
		if current == '.' || current == '-' || current == '_' || current == '/' ||
			(current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') || (current >= '0' && current <= '9') {
			index++
			continue
		}
		break
	}
	if index == start {
		return "", 0, fmt.Errorf("the condition has an unexpected %q", string(runes[start]))
	}
	return string(runes[start:index]), index, nil
}

// conditionParser 是递归下降的表达式解析器。
type conditionParser struct {
	tokens []conditionToken
	index  int
}

func (p *conditionParser) done() bool { return p.index >= len(p.tokens) }

func (p *conditionParser) peek() conditionToken { return p.tokens[p.index] }

func (p *conditionParser) parseOr() (conditionNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for !p.done() && p.peek().kind == tokenOr {
		p.index++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = conditionOr{left: left, right: right}
	}
	return left, nil
}

func (p *conditionParser) parseAnd() (conditionNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for !p.done() && p.peek().kind == tokenAnd {
		p.index++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = conditionAnd{left: left, right: right}
	}
	return left, nil
}

func (p *conditionParser) parseUnary() (conditionNode, error) {
	if !p.done() && p.peek().kind == tokenNot {
		p.index++
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return conditionNot{inner: inner}, nil
	}
	return p.parsePrimary()
}

func (p *conditionParser) parsePrimary() (conditionNode, error) {
	if p.done() {
		return nil, fmt.Errorf("the condition ends too early")
	}
	token := p.peek()
	if token.kind == tokenOpen {
		p.index++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.done() || p.peek().kind != tokenClose {
			return nil, fmt.Errorf("the condition has an unbalanced parenthesis")
		}
		p.index++
		return inner, nil
	}
	if token.kind != tokenValue {
		return nil, fmt.Errorf("the condition has an unexpected %q", token.text)
	}
	p.index++
	left := conditionOperand(token.text, token.quoted)
	if !p.done() && (p.peek().kind == tokenEqual || p.peek().kind == tokenNotEq) {
		operator := p.peek()
		p.index++
		if p.done() || p.peek().kind != tokenValue {
			return nil, fmt.Errorf("the condition needs a value after %q", operator.text)
		}
		right := conditionOperand(p.peek().text, p.peek().quoted)
		p.index++
		return conditionCompare{left: left, right: right, negated: operator.kind == tokenNotEq}, nil
	}
	return conditionTruthy{value: left}, nil
}

// conditionOperand 把一个记号变成操作数。事实路径带命名空间，因此事实路径
// 里一定有��点；没有点的裸词当作字面量，脚本作者不必给每个值加引号。
func conditionOperand(text string, quoted bool) conditionValue {
	if !quoted && strings.Contains(text, ".") {
		return conditionValue{path: text, isPath: true}
	}
	return conditionValue{literal: text}
}
