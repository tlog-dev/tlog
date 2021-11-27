package wire

import "fmt"

type (
	Tag byte
)

func (t Tag) String() string {
	switch t {
	case Int:
		return "Int"
	case Neg:
		return "Neg"
	case Bytes:
		return "Bytes"
	case String:
		return "String"
	case Array:
		return "Array"
	case Map:
		return "Map"
	case Semantic:
		return "Semantic"
	case Special:
		return "Special"
	default:
		return fmt.Sprintf("Tag(%x)", byte(t))
	}
}
