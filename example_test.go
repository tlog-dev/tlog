package tlog

func ExampleLogf() {
	Logf("simple message with args: %v %v %v", "str", 33, map[string]string{"a": "b"})
}

func ExampleStart() {
	s := Start()
	defer s.Finish()

	s.Logf("msg %v %v", "strval", 123)
}
