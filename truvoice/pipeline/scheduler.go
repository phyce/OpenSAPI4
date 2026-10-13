package pipeline

import "github.com/phyce/OpenSAPI4/truvoice/synthesizer"

// What the parameter generator's step reports.
const (
	ParametersFinished = iota // it has nothing left to do
	ParametersWaiting         // it waits for room in the frame ring
	ParametersWorking
)

// What Step reports to the speaking thread, as bits.
const (
	StepIdle     = 1 // the stages have nothing left to do until more text comes
	StepHandOver = 2 // hand the audio buffer over
)

// The free nodes the scheduler needs before it runs a stage.
const (
	inputFreeNodes      = 16
	normalizerFreeNodes = 38
	lexicalFreeNodes    = 38
	prosodyFreeNodes    = 106
)

// Stages are the stages the scheduler runs before the parameter generator.
type Stages interface {
	// Input makes the nodes of the next word from the escape parser's ring and reports whether there was one.
	Input() bool
	// Normalizer, Lexical and Prosody run their stage once and report whether it handed nodes on.
	Normalizer() bool
	Lexical() bool
	Prosody() bool
	// Reset resets the escape parser, the input stage, the normalizer, the lexical stage and prosody.
	Reset()
}

// Generator is the parameter generator, as the scheduler runs it.
type Generator interface {
	// Step runs it once and reports ParametersFinished, ParametersWaiting or ParametersWorking.
	Step() int
	// HasRoom reports whether the frame ring has room for the frames it waits to write.
	HasRoom() bool
	Reset()
}

// Step is one step of the scheduler: it runs the last stage that has work and enough free nodes, or the
// parameter generator and the synthesizer (pipeline.md §6). It reports StepIdle and StepHandOver bits.
func (pipeline *Pipeline) Step() int {
	if pipeline.ResetRequested {
		pipeline.ResetStages()
	}
	if pipeline.Retired != RetireDone {
		pipeline.Retired = pipeline.Retire()
	}
	if pipeline.WaitingForRoom && pipeline.Generator.HasRoom() {
		pipeline.WaitingForRoom = false
	}
	if !pipeline.WaitingForRoom && pipeline.HasWork[Parameters] {
		switch pipeline.Generator.Step() {
		case ParametersFinished:
			pipeline.HasWork[Parameters] = false
		case ParametersWaiting:
			pipeline.WaitingForRoom = true
		}
		if pipeline.Synthesizer.Run(func() { pipeline.Retired = pipeline.Retire() }) {
			return StepHandOver
		}
		return 0
	}
	stages := [Parameters]func() bool{pipeline.Stages.Normalizer, pipeline.Stages.Lexical, pipeline.Stages.Prosody}
	freeNodes := [Parameters]int{normalizerFreeNodes, lexicalFreeNodes, prosodyFreeNodes}
	for stage := Prosody; stage >= Normalizer; stage-- {
		if !pipeline.HasWork[stage] {
			continue
		}
		if pipeline.Free >= freeNodes[stage] {
			if stages[stage]() {
				pipeline.HasWork[stage+1] = true
			} else {
				pipeline.HasWork[stage] = false
			}
		}
		return 0
	}
	if pipeline.OutOfInput {
		if len(pipeline.Synthesizer.Samples) > 0 {
			return StepIdle | StepHandOver
		}
		return StepIdle
	}
	if pipeline.Free >= inputFreeNodes {
		if pipeline.Stages.Input() {
			pipeline.HasWork[Normalizer] = true
		} else {
			pipeline.OutOfInput = true
		}
	}
	return 0
}

// defaultSettings are the escape parser's settings on a new speaking thread.
var defaultSettings = Settings{Phrase: 1, Rate: 13, Pitch: 85, Flags: 0x17c0, InputFlags: 0x41}

const defaultSpeed = 150

// Initialize gives the escape parser's settings their defaults and resets the stages, as a new speaking
// thread does (pipeline.md §7).
func (pipeline *Pipeline) Initialize() {
	pipeline.Settings = defaultSettings
	pipeline.SettingsSpeed = defaultSpeed
	pipeline.SettingsLevels = pipeline.tables.GeneratedFrame
	pipeline.ResetStages()
}

// ResetStages empties the pipeline: the scheduler's flags, the nodes and the cursors, the frame ring, the
// synthesizer and every stage (pipeline.md §7).
func (pipeline *Pipeline) ResetStages() {
	pipeline.OutOfInput = true
	pipeline.HasWork = [Output]bool{}
	pipeline.WaitingForRoom = false
	pipeline.Retired = RetireDone
	pipeline.ResetRequested = false
	pipeline.ParsedReset = false
	pipeline.ResetNodes()
	initial := synthesizer.Frame(pipeline.tables.InitialFrame)
	pipeline.Synthesizer.ResetFrames(&initial)
	pipeline.Synthesizer.Reset(pipeline.SampleRate)
	pipeline.Levels = pipeline.SettingsLevels
	pipeline.Generator.Reset()
	pipeline.Stages.Reset()
}
