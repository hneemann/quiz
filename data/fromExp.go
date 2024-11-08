package data

import (
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/quiz/mathml"
)

func MathMlFromAST(a parser2.AST) (res mathml.Ast, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
			err = fmt.Errorf("error creating mathML: %w", err)
		}
	}()
	return _mathMlFromAST(a), nil
}

func _mathMlFromAST(a parser2.AST) mathml.Ast {
	switch v := a.(type) {
	case *parser2.Ident:
		return mathml.SimpleIdent(v.Name)
	case *parser2.Const[float64]:
		return mathml.SimpleNumber(fmt.Sprintf("%.6g", v.Value))
	case *parser2.Operate:
		switch v.Operator {
		case "/":
			return &mathml.Fraction{Top: _mathMlFromAST(v.A), Bottom: _mathMlFromAST(v.B)}
		case "^":
			return &mathml.Index{Base: checkBrace(v.A), Up: _mathMlFromAST(v.B)}
		default:
			if aop, ok := v.A.(*parser2.Operate); ok && (aop.Operator == "+" || aop.Operator == "-") && (v.Operator == "+" || v.Operator == "-") {
				return mergeRow(_mathMlFromAST(v.A), mathml.SimpleOperator(v.Operator), checkBrace(v.B))
			}
			if bop, ok := v.B.(*parser2.Operate); ok && (bop.Operator == "+" || bop.Operator == "-") && (v.Operator == "+") {
				return mergeRow(checkBrace(v.A), mathml.SimpleOperator(v.Operator), _mathMlFromAST(v.B))
			}
			return mergeRow(checkBrace(v.A), mathml.SimpleOperator(v.Operator), checkBrace(v.B))
		}
	case *parser2.FunctionCall:
		if id, ok := v.Func.(*parser2.Ident); ok {
			if id.Name == "sqrt" {
				return mathml.Sqrt{Inner: _mathMlFromAST(v.Args[0])}
			}
		}
		var list []mathml.Ast
		for i, ar := range v.Args {
			if i > 0 {
				list = append(list, mathml.SimpleOperator(","))
			}
			list = append(list, _mathMlFromAST(ar))
		}
		return mathml.NewRow(mathml.SimpleIdent(v.Func.String()), mathml.SimpleOperator("("), mathml.NewRow(list...), mathml.SimpleOperator(")"))
	default:
		panic(fmt.Errorf("unknown type %T", a))
	}
}

func checkBrace(a parser2.AST) mathml.Ast {
	if _, ok := a.(*parser2.Operate); ok {
		return mathml.NewRow(mathml.SimpleOperator("("), _mathMlFromAST(a), mathml.SimpleOperator(")"))
	} else {
		return _mathMlFromAST(a)
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
