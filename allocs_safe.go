// +build tlogsafe

package tlog

func encodeKVs0(e *Encoder, kvs []interface{}) {
	e.encodeKVs(kvs)
}
