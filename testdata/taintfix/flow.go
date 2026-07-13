package taintfix

func Source() string { return Mid("x") }
func Mid(s string) string { return Sink(s) }
func Sink(s string) string { return s }
