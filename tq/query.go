//go:build ignore

package tq

/*
	Expr       = Pipeline | MathExpr | Val
	Pipeline   = Filter { "|" Filter }
	Filter     = Ident [ ArgList ]
	ArgList    = "(" Expr { "," Expr } [ "," ] ")"
	Val        = Arr | Obj | Var | Literal
	Arr        = "[" Expr { "," Expr } [ "," ] "]"
	Obj        = "{" KV { "," KV } [ "," ] "}"
	KV         = Ident ":" Expr
	Var        = "$"IDENT
	Ident      = IDENT
	Literal    = LITERAL

	MathExpr   = OrExpr
	OrExpr     = AndExpr "||" AndExpr
	AndExpr    = MulExpr "&&" MulExpr
	MulExpr    = SumExpr MulOp SumExpr
	MulOp      = "*" | "/" | "%"
	SumExpr    = UnaryExpr SumOp UnaryExpr
	SumOp      = "+" | "-"
	UnaryExpr  = UnaryOp Var
	UnaryOp    = "-"
*/

func ParseQuery(q string) (Filter, error) {
	for i, c := range q {
		_ = i
		switch c {
		case ' ', '\t', '\n':
			continue
		case '"':
			//	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		}
	}

	return nil, nil
}
