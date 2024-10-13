package mathml

import (
	"bytes"
	"fmt"
	"io"
)

type Parser struct {
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

func NewRow(l ...Ast) Ast {
	if len(l) == 1 {
		return l[0]
	}
	return Row{l}
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

type Over struct {
	base Ast
	over Ast
}

func (o Over) ToMathMl(w io.Writer) {
	write(w, "<mover>")
	o.base.ToMathMl(w)
	o.over.ToMathMl(w)
	write(w, "</mover>")
}

type Under struct {
	base  Ast
	under Ast
}

func (o Under) ToMathMl(w io.Writer) {
	write(w, "<munder>")
	o.base.ToMathMl(w)
	o.under.ToMathMl(w)
	write(w, "</munder>")
}

type Sqrt struct {
	inner Ast
}

func (s Sqrt) ToMathMl(w io.Writer) {
	write(w, "<msqrt>")
	s.inner.ToMathMl(w)
	write(w, "</msqrt>")
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
	p := &Parser{tok: NewTokenizer(latex)}
	return p.Parse(EOF), nil
}

func (p *Parser) Parse(end Kind) Ast {
	var list []Ast
	for {
		tok := p.tok.NextToken()
		switch tok.kind {
		case end:
			return NewRow(list...)
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
			list[len(list)-1] = Index{list[len(list)-1], up, down}
		case Down:
			down := p.ParseBrace()
			var up Ast
			if p.tok.PeekToken().kind == Up {
				p.tok.NextToken()
				up = p.ParseBrace()
			}
			list[len(list)-1] = Index{list[len(list)-1], up, down}
		default:
			panic(fmt.Sprintf("unexpected token: %v", tok))
		}
	}
}

func (p *Parser) ParseCommand(value string) Ast {
	switch value {
	case "frac":
		top := p.ParseBrace()
		bottom := p.ParseBrace()
		return Fraction{top, bottom}
	case "pm":
		return SimpleOperator("&PlusMinus;")
	case "sqrt":
		return Sqrt{p.ParseBrace()}
	case "vec":
		return Over{base: p.ParseBrace(), over: SimpleOperator("&rarr;")}
	case "overset":
		over := p.ParseBrace()
		return Over{base: p.ParseBrace(), over: over}
	case "underset":
		under := p.ParseBrace()
		return Under{base: p.ParseBrace(), under: under}
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
		return SimpleNumber("&rightarrow;")
	case "alpha":
		return SimpleIdent("&alpha;")
	case "beta":
		return SimpleIdent("&beta;")
	case "Gamma":
		return SimpleIdent("&Gamma;")
	case "gamma":
		return SimpleIdent("&gamma;")
	case "Delta":
		return SimpleIdent("&Delta;")
	case "delta":
		return SimpleIdent("&delta;")
	case "Omega":
		return SimpleIdent("&Omega;")
	case "omega":
		return SimpleIdent("&omega;")
	case "Tau":
		return SimpleIdent("&Tau;")
	case "tau":
		return SimpleIdent("&tau;")
	case "Phi":
		return SimpleIdent("&Phi;")
	case "phi":
		return SimpleIdent("&phi;")
	case "Psi":
		return SimpleIdent("&Psi;")
	case "psi":
		return SimpleIdent("&psi;")
	case "Theta":
		return SimpleIdent("&Theta;")
	case "theta":
		return SimpleIdent("&theta;")
	case "chi":
		return SimpleIdent("&chi;")
	case "mu":
		return SimpleIdent("&mu;")
	case "epsilon":
		return SimpleIdent("&epsilon;")
	default:
		return SimpleIdent(value)
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

func (p *Parser) ParseBrace() Ast {
	n := p.tok.NextToken()
	if n.kind == Number || n.kind == Identifier {
		return SimpleItem{n}
	}
	if n.kind != OpenBrace {
		panic(fmt.Sprintf("unexpected token, expected {, got %v", n))
	}
	return p.Parse(CloseBrace)
}
