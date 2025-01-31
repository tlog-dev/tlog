package tlwire

import "math/big"

func (e *Encoder) AppendBigInt(b []byte, x *big.Int) []byte {
	b = e.AppendSemantic(b, Big)

	if x == nil {
		return e.AppendNil(b)
	}

	if x.Sign() >= 0 && x.BitLen() <= 64 {
		return e.AppendUint64(b, x.Uint64())
	}

	if x.BitLen() <= 63 {
		return e.AppendInt64(b, x.Int64())
	}

	b = e.AppendTag(b, String, 0)
	st := len(b)

	b = x.Append(b, 10)
	b = e.InsertLen(b, st, len(b)-st)

	return b
}

func (d *Decoder) BigInt(p []byte, st int, x *big.Int) (i int, err error) {
	if p[st] != Semantic|Big {
		panic("not a big")
	}

	tag, sub, i := d.Tag(p, st+1)
	if tag == Special && sub == Nil {
		x.SetInt64(0)
		return i, nil
	}
	if tag != String {
		panic("unsupported big encoding")
	}

	bs := p[i : i+int(sub)]
	i += int(sub)

	err = x.UnmarshalJSON(bs)
	if err != nil {
		return st, err
	}

	return i, nil
}
