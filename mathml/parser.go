package mathml

import (
	"bytes"
	"fmt"
	"io"
)

type parser struct {
	tok *Tokenizer
}

type Ast interface {
	ToMathMl(w io.Writer)
}

type SimpleItem struct {
	tok Token
}

func (s SimpleItem) ToMathMl(w io.Writer) {
	switch s.tok.kind {
	case Number:
		write(w, "<mn>", s.tok.value, "</mn>")
	case Identifier:
		write(w, "<mi>", s.tok.value, "</mi>")
	default:
		write(w, "<mo>", s.tok.value, "</mo>")
	}
}

func write(w io.Writer, s ...string) {
	for _, ss := range s {
		w.Write([]byte(ss))
	}
}

type Row struct {
	items []Ast
}

func (f Row) ToMathMl(w io.Writer) {
	write(w, "<mrow>")
	for _, item := range f.items {
		item.ToMathMl(w)
	}
	write(w, "</mrow>")
}

type Empty struct {
}

func (e Empty) ToMathMl(io.Writer) {
}

func NewRow(l ...Ast) Ast {
	switch len(l) {
	case 0:
		return Empty{}
	case 1:
		return l[0]
	default:
		return Row{l}
	}
}

type Fraction struct {
	top    Ast
	bottom Ast
}

func (f Fraction) ToMathMl(w io.Writer) {
	write(w, "<mfrac>")
	f.top.ToMathMl(w)
	f.bottom.ToMathMl(w)
	write(w, "</mfrac>")
}

type Index struct {
	base Ast
	up   Ast
	down Ast
}

func (i Index) ToMathMl(w io.Writer) {
	if i.up == nil {
		write(w, "<msub>")
		i.base.ToMathMl(w)
		i.down.ToMathMl(w)
		write(w, "</msub>")
		return
	}
	if i.down == nil {
		write(w, "<msup>")
		i.base.ToMathMl(w)
		i.up.ToMathMl(w)
		write(w, "</msup>")
		return
	}
	write(w, "<msubsup>")
	i.base.ToMathMl(w)
	i.down.ToMathMl(w)
	i.up.ToMathMl(w)
	write(w, "</msubsup>")
}

func NewIndex(base Ast, up Ast, down Ast) Ast {
	if i, ok := base.(SimpleItem); ok && i.tok.kind == Operator {
		return UnderOver{base: base, over: up, under: down}
	}
	return Index{base: base, up: up, down: down}
}

type UnderOver struct {
	base  Ast
	over  Ast
	under Ast
}

func (o UnderOver) ToMathMl(w io.Writer) {
	if o.over == nil {
		write(w, "<munder>")
		o.base.ToMathMl(w)
		o.under.ToMathMl(w)
		write(w, "</munder>")
		return
	}
	if o.under == nil {
		write(w, "<mover>")
		o.base.ToMathMl(w)
		o.over.ToMathMl(w)
		write(w, "</mover>")
		return
	}
	write(w, "<munderover>")
	o.base.ToMathMl(w)
	o.under.ToMathMl(w)
	o.over.ToMathMl(w)
	write(w, "</munderover>")
}

type Sqrt struct {
	inner Ast
}

func (s Sqrt) ToMathMl(w io.Writer) {
	write(w, "<msqrt>")
	s.inner.ToMathMl(w)
	write(w, "</msqrt>")
}

type align int

const (
	left align = iota
	center
	right
)

type cellStyle struct {
	leftBorder  bool
	rightBorder bool
	align       align
}

func (s cellStyle) style() string {
	style := ""
	if s.leftBorder {
		style += "border-left:1px solid black;"
	}
	if s.rightBorder {
		style += "border-right:1px solid black;"
	}
	switch s.align {
	case left:
		style += "text-align:left;"
	case right:
		style += "text-align:right;"
	}
	return style
}

type Table struct {
	table [][]Ast
	style []cellStyle
}

func (t Table) ToMathMl(w io.Writer) {
	write(w, "<mtable>")
	topLine := false
	for _, row := range t.table {
		write(w, "<mtr>")
		if len(row) == 0 {
			topLine = true
		} else {
			for i, item := range row {
				style := ""
				if i < len(t.style) {
					style = t.style[i].style()
				}
				if topLine {
					style += "border-top:1px solid black;"
				}
				if style != "" {
					write(w, "<mtd style=\"", style, "\">")
				} else {
					write(w, "<mtd>")
				}
				item.ToMathMl(w)
				write(w, "</mtd>")
			}
			topLine = false
		}
		write(w, "</mtr>")
	}
	write(w, "</mtable>")
}

func ScanDollar(text string) string {
	out := bytes.Buffer{}
	math := bytes.Buffer{}
	inMath := false
	for _, r := range text {
		if r == '$' {
			if inMath {
				m := math.String()
				if m != "" {
					mathml, err := LaTeXtoMathMLString(m)
					if err != nil {
						out.WriteString("<i>")
						out.WriteString(err.Error())
						out.WriteString("</i>")
					} else {
						out.WriteString("<math xmlns=\"&mathml;\">")
						out.WriteString(mathml)
						out.WriteString("</math>")
					}
				} else {
					out.WriteString("$")
				}
				math.Reset()
			}
			inMath = !inMath
			continue
		}
		if inMath {
			math.WriteRune(r)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func LaTeXtoMathMLString(latex string) (string, error) {
	ast, err := ParseLaTeX(latex)
	if err != nil {
		return "", err
	}
	b := bytes.Buffer{}
	ast.ToMathMl(&b)
	return b.String(), err
}

func ParseLaTeX(latex string) (ast Ast, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("parse error: %v", r)
			}
		}
	}()
	p := &parser{tok: NewTokenizer(latex)}
	return p.Parse(EOF), nil
}

func (p *parser) Parse(end Kind) Ast {
	a, _ := p.ParseFunc(func(t Token) bool { return t.kind == end })
	return a
}

func (p *parser) ParseFunc(isEnd func(token Token) bool) (Ast, Token) {
	var list []Ast
	for {
		tok := p.tok.NextToken()
		if isEnd(tok) {
			return NewRow(list...), tok
		}
		switch tok.kind {
		case Number:
			list = append(list, SimpleItem{tok: tok})
		case Identifier:
			list = append(list, SimpleItem{tok: tok})
		case Operator:
			list = append(list, SimpleItem{tok: tok})
		case OpenParen:
			inner := p.Parse(CloseParen)
			list = append(list, NewRow(SimpleOperator("("), inner, SimpleOperator(")")))
		case Command:
			list = append(list, p.ParseCommand(tok.value))
		case Up:
			up := p.ParseBrace()
			var down Ast
			if p.tok.PeekToken().kind == Down {
				p.tok.NextToken()
				down = p.ParseBrace()
			}
			list[len(list)-1] = NewIndex(list[len(list)-1], up, down)
		case Down:
			down := p.ParseBrace()
			var up Ast
			if p.tok.PeekToken().kind == Up {
				p.tok.NextToken()
				up = p.ParseBrace()
			}
			list[len(list)-1] = NewIndex(list[len(list)-1], up, down)
		default:
			panic(fmt.Sprintf("unexpected token: %v", tok))
		}
	}
}

func (p *parser) ParseCommand(value string) Ast {
	switch value {
	case "frac":
		top := p.ParseBrace()
		bottom := p.ParseBrace()
		return Fraction{top, bottom}
	case "pm":
		return SimpleOperator("&PlusMinus;")
	case "left":
		open := p.tok.NextToken()
		if !(open.kind == OpenParen || open.kind == CloseParen || open.kind == Operator) {
			panic(fmt.Sprintf("unexpected token: %v", open))
		}
		inner, _ := p.ParseFunc(func(t Token) bool { return t.kind == Command && t.value == "right" })
		clo := p.tok.NextToken()
		if !(clo.kind == OpenParen || clo.kind == CloseParen || clo.kind == Operator) {
			panic(fmt.Sprintf("unexpected token: %v", clo))
		}
		return NewRow(SimpleOperator(open.value), inner, SimpleOperator(clo.value))
	case "sqrt":
		return Sqrt{p.ParseBrace()}
	case "vec":
		return UnderOver{base: p.ParseBrace(), over: SimpleOperator("&rarr;")}
	case "table":
		return p.parseTable()
	case "overset":
		over := p.ParseBrace()
		return UnderOver{base: p.ParseBrace(), over: over}
	case "underset":
		under := p.ParseBrace()
		return UnderOver{base: p.ParseBrace(), under: under}
	case "sum":
		return SimpleOperator("&sum;")
	case "int":
		return SimpleOperator("&int;")
	case "oint":
		return SimpleOperator("&oint;")
	case "cdot":
		return SimpleOperator("&middot;")
	case "dif":
		return SimpleNumber("d")
	case "infty":
		return SimpleNumber("&infin;")
	case "rightarrow":
		return SimpleOperator("&rightarrow;")
	case "Rightarrow":
		return SimpleOperator("&Rightarrow;")
	case "sin":
		return SimpleIdent("sin")
	case "cos":
		return SimpleIdent("cos")
	case "tan":
		return SimpleIdent("tan")
	case "ln":
		return SimpleIdent("ln")
	case "lim":
		return SimpleIdent("lim")
	default:
		// assuming it's a symbol
		return SimpleIdent("&" + value + ";")
	}
}

func SimpleIdent(s string) Ast {
	return SimpleItem{Token{kind: Identifier, value: s}}
}
func SimpleOperator(s string) Ast {
	return SimpleItem{Token{kind: Operator, value: s}}
}
func SimpleNumber(s string) Ast {
	return SimpleItem{Token{kind: Number, value: s}}
}

func (p *parser) ParseBrace() Ast {
	n := p.tok.NextToken()
	if n.kind == Number || n.kind == Identifier {
		return SimpleItem{n}
	}
	if n.kind != OpenBrace {
		panic(fmt.Sprintf("unexpected token, expected {, got %v", n))
	}
	return p.Parse(CloseBrace)
}

func (p *parser) parseTable() Ast {
	n := p.tok.NextToken()
	var styles []cellStyle
	if n.kind == Operator && n.value == "[" {
		styles = p.parseTableDef()
		n = p.tok.NextToken()
	}

	if n.kind != OpenBrace {
		panic(fmt.Sprintf("unexpected token, expected {, got %v", n))
	}

	var table [][]Ast
	var row []Ast
	for {
		a, tok := p.ParseFunc(func(t Token) bool { return t.kind == CloseBrace || t.kind == Ampersand || t.kind == Linefeed })
		row = append(row, a)
		switch tok.kind {
		case CloseBrace:
			if len(row) > 0 {
				table = append(table, row)
			}
			return Table{table: table, style: styles}
		case Linefeed:
			if len(row) == 1 {
				if _, ok := row[0].(Empty); ok {
					row = nil
				}
			}
			table = append(table, row)
			row = nil
		}
	}
}

func (p *parser) parseTableDef() []cellStyle {
	var styles []cellStyle
	complete := false
	cs := cellStyle{}
	for {
		switch p.tok.Read() {
		case ']':
			if complete {
				styles = append(styles, cs)
			}
			return styles
		case '|':
			if complete {
				cs.rightBorder = true
				styles = append(styles, cs)
				cs = cellStyle{}
				complete = false
			} else {
				cs.leftBorder = true
			}
		case 'l':
			if complete {
				styles = append(styles, cs)
				cs = cellStyle{}
			}
			complete = true
			cs.align = left
		case 'r':
			if complete {
				styles = append(styles, cs)
				cs = cellStyle{}
			}
			complete = true
			cs.align = right
		case 'c':
			if complete {
				styles = append(styles, cs)
				cs = cellStyle{}
			}
			complete = true
			cs.align = center
		}
	}
}