package tlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructured(t *testing.T) {
	a := StructuredConfig{
		MessageWidth:     1,
		ValueMaxPadWidth: 3,

		PairSeparator: "4",
		KVSeparator:   "5",

		QuoteAnyValue:   true,
		QuoteEmptyValue: true,
	}

	b := a.Copy()

	assert.Equal(t, &a, &b)
}
