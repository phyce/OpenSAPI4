package input

// Output is what the last pump gave the escape parser.
func (stage *Stage) Output() []byte {
	return stage.output
}
