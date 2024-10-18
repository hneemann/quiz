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
	ToMathMl(w io.Writer, attr map[string]string)
}

type SimpleItem struct {
	tok      Token
	fontsize string
}

func (s SimpleItem) setFontSize(size string) SimpleItem {
	s.fontsize = size
	return s
}

func (s SimpleItem) ToMathMl(w io.Writer, attr map[string]string) {
	switch s.tok.kind {
	case Number:
		s.write(w, "mn", attr)
	case Identifier:
		s.write(w, "mi", attr)
	default:
		s.write(w, "mo", attr)
	}
}
func (s SimpleItem) write(w io.Writer, t string, attr map[string]string) {
	tag(w, t, attr, func(w io.Writer) {
		write(w, s.tok.value)
	})
}

func write(w io.Writer, s ...string) {
	for _, ss := range s {
		w.Write([]byte(ss))
	}
}

func tag(w io.Writer, tag string, attr map[string]string, inner func(w io.Writer)) {
	if attr == nil {
		write(w, "<", tag, ">")
	} else {
		write(w, "<", tag)
		for k, v := range attr {
			write(w, " ", k, "=\"", v, "\"")
		}
		write(w, ">")
	}
	inner(w)
	write(w, "</", tag, ">")
}

type Row struct {
	items []Ast
}

func (f Row) ToMathMl(w io.Writer, attr map[string]string) {
	tag(w, "mrow", attr, func(w io.Writer) {
		for _, item := range f.items {
			item.ToMathMl(w, nil)
		}
	})
}

type Empty struct {
}

func (e Empty) ToMathMl(io.Writer, map[string]string) {
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

type AddAttribute struct {
	inner Ast
	attr  map[string]string
}

func (a AddAttribute) ToMathMl(w io.Writer, attr map[string]string) {
	a.inner.ToMathMl(w, a.attr)
}

func addAttribute(key, value string, inner Ast) Ast {
	if a, ok := inner.(AddAttribute); ok {
		a.attr[key] = value
		return a
	}
	return AddAttribute{inner: inner, attr: map[string]string{key: value}}
}

type Fraction struct {
	top    Ast
	bottom Ast
}

func (f Fraction) ToMathMl(w io.Writer, attr map[string]string) {
	tag(w, "mfrac", attr, func(w io.Writer) {
		f.top.ToMathMl(w, nil)
		f.bottom.ToMathMl(w, nil)
	})
}

type Index struct {
	base Ast
	up   Ast
	down Ast
}

func (i Index) ToMathMl(w io.Writer, attr map[string]string) {
	if i.up == nil {
		tag(w, "msub", attr, func(w io.Writer) {
			i.base.ToMathMl(w, nil)
			i.down.ToMathMl(w, nil)
		})
		return
	}
	if i.down == nil {
		tag(w, "msup", attr, func(w io.Writer) {
			i.base.ToMathMl(w, nil)
			i.up.ToMathMl(w, nil)
		})
		return
	}
	tag(w, "msubsup", attr, func(w io.Writer) {
		i.base.ToMathMl(w, nil)
		i.down.ToMathMl(w, nil)
		i.up.ToMathMl(w, nil)
	})
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

func (o UnderOver) ToMathMl(w io.Writer, attr map[string]string) {
	if o.over == nil {
		tag(w, "munder", attr, func(w io.Writer) {
			o.base.ToMathMl(w, nil)
			o.under.ToMathMl(w, nil)
		})
		return
	}
	if o.under == nil {
		tag(w, "mover", attr, func(w io.Writer) {
			o.base.ToMathMl(w, nil)
			o.over.ToMathMl(w, nil)
		})
		return
	}
	tag(w, "munderover", attr, func(w io.Writer) {
		o.base.ToMathMl(w, nil)
		o.under.ToMathMl(w, nil)
		o.over.ToMathMl(w, nil)
	})
}

type Sqrt struct {
	inner Ast
}

func (s Sqrt) ToMathMl(w io.Writer, attr map[string]string) {
	tag(w, "msqrt", attr, func(w io.Writer) {
		s.inner.ToMathMl(w, nil)
	})
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

func (t Table) ToMathMl(w io.Writer, attr map[string]string) {
	tag(w, "mtable", attr, func(w io.Writer) {
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
					item.ToMathMl(w, nil)
					write(w, "</mtd>")
				}
				topLine = false
			}
			write(w, "</mtr>")
		}
	})
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
	ast.ToMathMl(&b, nil)
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
			up := p.ParseInBrace()
			var down Ast
			if p.tok.PeekToken().kind == Down {
				p.tok.NextToken()
				down = p.ParseInBrace()
			}
			list[len(list)-1] = NewIndex(list[len(list)-1], up, down)
		case Down:
			down := p.ParseInBrace()
			var up Ast
			if p.tok.PeekToken().kind == Up {
				p.tok.NextToken()
				up = p.ParseInBrace()
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
		top := p.ParseInBrace()
		bottom := p.ParseInBrace()
		return Fraction{top, bottom}
	case "pm":
		return SimpleOperator("&PlusMinus;")
	case "left":
		open := p.getBrace(OpenParen)
		inner, _ := p.ParseFunc(func(t Token) bool { return t.kind == Command && t.value == "right" })
		close := p.getBrace(CloseParen)
		return NewRow(open, inner, close)
	case "sqrt":
		return Sqrt{p.ParseInBrace()}
	case "vec":
		return UnderOver{base: p.ParseInBrace(), over: addAttribute("mathsize", "75%", SimpleOperator("&rarr;"))}
	case "u":
		return SimpleNumber(p.ParsePlainInBrace())
	case "table":
		return p.parseTable()
	case "overset":
		over := p.ParseInBrace()
		return UnderOver{base: p.ParseInBrace(), over: over}
	case "ds":
		inner := p.ParseInBrace()
		return addAttribute("displaystyle", "true", inner)
	case "underset":
		under := p.ParseInBrace()
		return UnderOver{base: p.ParseInBrace(), under: under}
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
	case "leftarrow":
		return SimpleOperator("&leftarrow;")
	case "Leftarrow":
		return SimpleOperator("&Leftarrow;")
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

func (p *parser) getBrace(brace Kind) Ast {
	b := p.tok.NextToken()
	if !(b.kind == brace || b.kind == Operator) {
		panic(fmt.Sprintf("unexpected token behind \\left or \\right: %v", b))
	}
	if b.value == "." {
		return Empty{}
	}
	return SimpleOperator(b.value)
}

func SimpleIdent(s string) Ast {
	return SimpleItem{tok: Token{kind: Identifier, value: s}}
}
func SimpleOperator(s string) Ast {
	return SimpleItem{tok: Token{kind: Operator, value: s}}
}
func SimpleNumber(s string) Ast {
	return SimpleItem{tok: Token{kind: Number, value: s}}
}

func (p *parser) ParseInBrace() Ast {
	n := p.tok.NextToken()
	if n.kind == Number || n.kind == Identifier {
		return SimpleItem{tok: n}
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
		panic(fmt.Sprintf("unexpected token behind \\table, expected {, got %v", n))
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
				if si, ok := row[0].(SimpleItem); ok && si.tok.value == "-" {
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

func (p *parser) ParsePlainInBrace() string {
	n := p.tok.NextToken()
	if n.kind != OpenBrace {
		panic(fmt.Sprintf("unexpected token, expected {, got %v", n))
	}
	var text string
	for {
		n = p.tok.NextToken()
		if n.kind == CloseBrace || n.kind == EOF {
			return text
		}
		text += n.value
	}
}
