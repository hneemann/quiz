package data

import (
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/quiz/mathml"
)

func MathMlFromAST(a parser2.AST) mathml.Ast {
	switch v := a.(type) {
	case *parser2.Ident:
		return mathml.SimpleIdent(v.Name)
	case *parser2.Const[float64]:
		return mathml.SimpleNumber(fmt.Sprintf("%.6g", v.Value))
	case *parser2.Operate:
		if v.Operator == "/" {
			return &mathml.Fraction{Top: MathMlFromAST(v.A), Bottom: MathMlFromAST(v.B)}
		}
		if aop, ok := v.A.(*parser2.Operate); ok && (aop.Operator == "+" || aop.Operator == "-") && (v.Operator == "+" || v.Operator == "-") {
			return mergeRow(MathMlFromAST(v.A), mathml.SimpleOperator(v.Operator), checkBrace(v.B))
		}
		if bop, ok := v.B.(*parser2.Operate); ok && (bop.Operator == "+" || bop.Operator == "-") && (v.Operator == "+") {
			return mergeRow(checkBrace(v.A), mathml.SimpleOperator(v.Operator), MathMlFromAST(v.B))
		}
		return mergeRow(checkBrace(v.A), mathml.SimpleOperator(v.Operator), checkBrace(v.B))
	}
	return nil
}

func checkBrace(a parser2.AST) mathml.Ast {
	if _, ok := a.(*parser2.Operate); ok {
		return mathml.NewRow(mathml.SimpleOperator("("), MathMlFromAST(a), mathml.SimpleOperator(")"))
	} else {
		return MathMlFromAST(a)
	}
}

func mergeRow(list ...mathml.Ast) mathml.Ast {
	var l []mathml.Ast
	for _, a := range list {
		if a == nil {
			continue
		}
		if r, ok := a.(*mathml.Row); ok {
			l = append(l, r.Items()...)
		} else {
			l = append(l, a)
		}
	}
	return mathml.NewRow(l...)
}
