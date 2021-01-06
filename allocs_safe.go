// +build tlogsafe

package tlog

func append0(b []interface{}, v ...interface{}) []interface{} {
	return append(b, v...)
}

func encodeKVs0(e *Encoder, kvs ...interface{}) {
	e.encodeKVs(kvs...)
}
